package bench

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
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
	totalTickets            *atomic.Int64
	refundWg                *sync.WaitGroup
	criticalError           chan error
	currentTicketPhaseIndex *atomic.Int32
	currentSalesPhaseIndex  *atomic.Int32
}

type InitializeResponse struct {
	InitializedAt time.Time `json:"initialized_at"`
	AppLanguage   string    `json:"app_language"`
	UiLanguage    string    `json:"ui_language"`
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

	agent, err := agent.NewAgent(agent.WithBaseURL(targetURL), agent.WithDefaultTransport())
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

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Initialize atomic counters for sales and refunds
	var totalSales atomic.Int64
	var totalRefunds atomic.Int64
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

	worker, err := worker.NewWorker(func(ctx context.Context, _ int) {
		scenario.RunUserScenario(ctx)
	}, worker.WithMaxParallelism(8))
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
	finalTickets := totalTickets.Load()
	finalTicketPhase := currentTicketPhaseIndex.Load()
	finalSalesPhase := currentSalesPhaseIndex.Load()
	currentTimeStr = getApplicationClock(scenario.initializedAt)
	slog.Info("Benchmark Finished!",
		"score", int64((finalSales-finalRefunds)/100),
		"total_sales", finalSales,
		"total_refunds", finalRefunds,
		"net_revenue", finalSales-finalRefunds,
		"total_tickets", finalTickets,
		"ticket_phase", fmt.Sprintf("%d/%d", finalTicketPhase, len(ticketSoldPhases)),
		"sales_phase", fmt.Sprintf("%d/%d", finalSalesPhase, len(salesPhases)),
		"current_time", currentTimeStr)
}
