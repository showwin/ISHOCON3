package bench

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/context"

	"github.com/showwin/ISHOCON3/benchmark/bench/logger"

	"github.com/isucon/isucandar/agent"
	"github.com/isucon/isucandar/worker"
)

var jst = time.FixedZone("Asia/Tokyo", 9*60*60)

type NewUserCount struct {
	Count int
}

type Scenario struct {
	targetURL               string
	initializedAt           time.Time
	log                     logger.Logger
	totalSales              *atomic.Int64
	totalRefunds            *atomic.Int64
	totalPurchased          *atomic.Int64
	totalTickets            *atomic.Int64
	refundWg                *sync.WaitGroup
	criticalError           chan error
	currentTicketPhaseIndex *atomic.Int32
	currentSalesPhaseIndex  *atomic.Int32
}

type InitializeResponse struct {
	InitializedAt time.Time `json:"initialized_at"`
	AppLanguage   string    `json:"app_language"`
}

// type BoughtSeat struct {
//   ScheduleId  string
//   Seat        string
//   StationFrom string
//   StationTo   string
// }

// type Score struct {
//   Expense int
//   Refund  int
// }

func Run(targetURL string, logLevel string) {
	rand.New(rand.NewSource(time.Now().UnixNano())) // Seed random number generator

	agent, err := agent.NewAgent(agent.WithBaseURL(targetURL), agent.WithTimeout(5*time.Second), agent.WithDefaultTransport())
	if err != nil {
		slog.Error("failed to create agent", "error", err.Error())
	}
	httpResp, err := HttpPost(context.Background(), agent, "/api/initialize", nil)
	if err != nil {
		slog.Error("failed to post /initialize", "error", err.Error())
	}
	var initResp InitializeResponse
	if err := json.Unmarshal(httpResp.Body, &initResp); err != nil {
		slog.Error("failed to unmarshal response", "error", err.Error())
	}
	if httpResp.StatusCode != 200 {
		slog.Error("initialize returned non-200 status", "status_code", httpResp.StatusCode, "body", string(httpResp.Body))
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Initialize atomic counters for sales and refunds
	var totalSales atomic.Int64
	var totalRefunds atomic.Int64
	var totalPurchased atomic.Int64
	var totalTickets atomic.Int64
	var currentTicketPhaseIndex atomic.Int32
	var currentSalesPhaseIndex atomic.Int32
	var refundWg sync.WaitGroup
	criticalError := make(chan error, 1) // Buffered channel to prevent blocking

	log := logger.GetLogger(logLevel)
	scenario := Scenario{
		targetURL:               targetURL,
		initializedAt:           initResp.InitializedAt,
		log:                     log,
		totalSales:              &totalSales,
		totalRefunds:            &totalRefunds,
		totalPurchased:          &totalPurchased,
		totalTickets:            &totalTickets,
		refundWg:                &refundWg,
		criticalError:           criticalError,
		currentTicketPhaseIndex: &currentTicketPhaseIndex,
		currentSalesPhaseIndex:  &currentSalesPhaseIndex,
	}

	currentTimeStr := getApplicationClock(scenario.initializedAt)
	slog.Info("Benchmark Start!", "current_time", currentTimeStr)

	// Start admin scenario
	go scenario.RunAdminScenario(ctx)

	// Define worker counts per sales phase.
	// <sales> => <total workers>
	// 0 => 15
	// 1000 => 20
	// 3000 => 25
	// 10000 => 30
	// 50000 => 50
	// 200000 => 100
	// 500000 => 200
	// 1000000 => 300
	phaseWorkerCounts := []int{15, 20, 25, 30, 50, 100, 200, 300}

	// Calculate total workers needed
	totalWorkers := phaseWorkerCounts[len(phaseWorkerCounts)-1]

	// Use atomic counter to assign worker IDs since workerID from isucandar is -1 with InfinityLoop
	var workerIDCounter atomic.Int32

	// Monitor sales phase changes and log ad campaign launches
	go func() {
		lastPhase := int32(-1)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				currentPhase := scenario.currentSalesPhaseIndex.Load()
				if currentPhase != lastPhase && currentPhase > 0 && int(currentPhase) < len(phaseWorkerCounts) {
					newWorkers := phaseWorkerCounts[currentPhase]
					oldWorkers := phaseWorkerCounts[currentPhase-1]

					if newWorkers > oldWorkers {
						currentTimeStr := getApplicationClock(scenario.initializedAt)
						slog.Info("New ad campaign launched!",
							"sales_phase", fmt.Sprintf("%d/%d", currentPhase, len(salesPhases)),
							"active_buyers", newWorkers,
							"previous_buyers", oldWorkers,
							"current_time", currentTimeStr,
							"user", "admin",
						)
					}
					lastPhase = currentPhase
				}
			}
		}
	}()

	worker, err := worker.NewWorker(func(ctx context.Context, _ int) {
		// Assign a unique ID to this worker goroutine
		myWorkerID := int(workerIDCounter.Add(1) - 1)

		// Determine which phase this worker belongs to
		// Workers 0-9 belong to phase 0 (10 workers)
		// Workers 10-14 belong to phase 1 (15 workers total)
		// Workers 15-19 belong to phase 2 (20 workers total), etc.
		workerPhase := len(phaseWorkerCounts) - 1 // Default to last phase
		for i := 0; i < len(phaseWorkerCounts); i++ {
			if myWorkerID < phaseWorkerCounts[i] {
				workerPhase = i
				break
			}
		}

		// Wait until this worker's phase is reached
		for {
			currentPhase := scenario.currentSalesPhaseIndex.Load()
			if currentPhase >= int32(workerPhase) {
				break
			}

			// Check if context is cancelled
			select {
			case <-ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
				// Continue waiting
			}
		}

		// Run the user scenario
		scenario.RunUserScenario(ctx)
	}, worker.WithInfinityLoop(), worker.WithMaxParallelism(int32(totalWorkers)))
	if err != nil {
		panic(err)
	}

	// Run worker in a goroutine so we can handle critical errors
	workerDone := make(chan struct{})
	go func() {
		worker.Process(ctx)
		close(workerDone)
	}()

	// Wait for either worker completion or critical error
	select {
	case <-workerDone:
		// Normal completion
	case critErr := <-criticalError:
		// Critical error occurred, cancel context and stop benchmark
		slog.Error("Critical error occurred, stopping benchmark", "error", critErr.Error())
		cancel()
		// Wait a bit for goroutines to clean up
		<-workerDone
	}

	currentTimeStr = getApplicationClock(scenario.initializedAt)
	slog.Info("Main phase finished. Waiting for pending refunds to complete...", "current_time", currentTimeStr)

	// Wait for all refund operations to complete
	refundWg.Wait()

	// Check if there was a critical error during refund phase
	select {
	case critErr := <-criticalError:
		slog.Error("Critical error occurred, stopping benchmark", "error", critErr.Error())
		panic(critErr)
	default:
		// No critical error, proceed with final score
	}

	finalSales := totalSales.Load()
	finalRefunds := totalRefunds.Load()
	finalPurchased := totalPurchased.Load()
	finalTickets := totalTickets.Load()
	finalTicketPhase := currentTicketPhaseIndex.Load()
	finalSalesPhase := currentSalesPhaseIndex.Load()
	currentTimeStr = getApplicationClock(scenario.initializedAt)

	score := int64((float64(finalSales) + float64(finalPurchased-finalSales)*0.5 - float64(finalRefunds)) / 100)

	slog.Info("Benchmark Finished!",
		"score", score,
		"total_sales", finalSales,
		"total_purchased", finalPurchased,
		"total_refunds", finalRefunds,
		"net_revenue", finalSales-finalRefunds,
		"total_tickets", finalTickets,
		"ticket_phase", fmt.Sprintf("%d/%d", finalTicketPhase, len(ticketSoldPhases)),
		"sales_phase", fmt.Sprintf("%d/%d", finalSalesPhase, len(salesPhases)),
		"current_time", currentTimeStr)
}
