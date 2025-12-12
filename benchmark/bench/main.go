package bench

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/context"

	"github.com/showwin/ISHOCON3/benchmark/bench/logger"

	"github.com/isucon/isucandar/agent"
)

var jst = time.FixedZone("Asia/Tokyo", 9*60*60)

type NewUserCount struct {
	Count int
}

type Scenario struct {
	targetURL               string
	initializedAt           time.Time
	appLanguage             string
	log                     logger.Logger
	totalSales              *[32]atomic.Int64
	totalRefunds            *[32]atomic.Int64
	totalPurchased          *[32]atomic.Int64
	totalTickets            *[32]atomic.Int64
	refundWg                *sync.WaitGroup
	criticalError           chan error
	currentTicketPhaseIndex *atomic.Int32
	currentSalesPhaseIndex  *atomic.Int32
	addWorkersFn            func(ticketPhase, salesPhase int32)
	purchasedReservations   *sync.Map // key: unique ID, value: "ScheduleID|Seat|FromTo" (e.g., "E2123|A-3|AD")
	ticketPhaseChans        []chan struct{}
	salesPhaseChans         []chan struct{}
}

// sumShardedCounter sums all 32 shards of a counter
func sumShardedCounter(counter *[32]atomic.Int64) int64 {
	var total int64
	for i := 0; i < 32; i++ {
		total += counter[i].Load()
	}
	return total
}

type InitializeResponse struct {
	InitializedAt time.Time `json:"initialized_at"`
	AppLanguage   string    `json:"app_language"`
}

func Run(targetURL string, logLevel string) {
	// Limit to 4 CPU cores for benchmark consistency
	runtime.GOMAXPROCS(4)

	rand.New(rand.NewSource(time.Now().UnixNano())) // Seed random number generator

	agent, err := agent.NewAgent(agent.WithBaseURL(targetURL), agent.WithTimeout(10*time.Second), agent.WithDefaultTransport())
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

	// Initialize sharded atomic counters to reduce contention
	var totalSales [32]atomic.Int64
	var totalRefunds [32]atomic.Int64
	var totalPurchased [32]atomic.Int64
	var totalTickets [32]atomic.Int64
	var currentTicketPhaseIndex atomic.Int32
	var currentSalesPhaseIndex atomic.Int32
	var refundWg sync.WaitGroup
	criticalError := make(chan error, 1) // Buffered channel to prevent blocking
	var purchasedReservations sync.Map   // Stores "ScheduleID|Seat|FromTo" strings

	// Phase channels for controlling pre-spawned workers (much faster than flag polling)
	ticketPhaseChans := make([]chan struct{}, 6)
	ticketPhaseOnce := make([]sync.Once, 6)
	for i := range ticketPhaseChans {
		ticketPhaseChans[i] = make(chan struct{})
	}
	salesPhaseChans := make([]chan struct{}, 8)
	salesPhaseOnce := make([]sync.Once, 8)
	for i := range salesPhaseChans {
		salesPhaseChans[i] = make(chan struct{})
	}

	log := logger.GetLogger(logLevel)
	scenario := Scenario{
		targetURL:               targetURL,
		initializedAt:           initResp.InitializedAt,
		appLanguage:             initResp.AppLanguage,
		log:                     log,
		totalSales:              &totalSales,
		totalRefunds:            &totalRefunds,
		totalPurchased:          &totalPurchased,
		totalTickets:            &totalTickets,
		refundWg:                &refundWg,
		criticalError:           criticalError,
		currentTicketPhaseIndex: &currentTicketPhaseIndex,
		currentSalesPhaseIndex:  &currentSalesPhaseIndex,
		purchasedReservations:   &purchasedReservations,
		ticketPhaseChans:        ticketPhaseChans,
		salesPhaseChans:         salesPhaseChans,
	}

	currentTimeStr := getApplicationClock(scenario.initializedAt)
	slog.Info("Benchmark Start!", "current_time", currentTimeStr)

	// Start admin scenario
	go scenario.RunAdminScenario(ctx)

	// Define worker counts per ticket phase.
	// <tickets> => <added workers>
	// 0 => 5
	// 5 => +5
	// 10 => +10
	// 50 => +20
	// 100 => +20
	// 200 => +20
	ticketPhaseWorkerCounts := []int{5, 5, 10, 20, 20, 20}

	// Define worker counts per sales phase.
	// <sales> => <added workers>
	// 0 => 10
	// 1000 => +5
	// 3000 => +5
	// 10000 => +5
	// 50000 => +20
	// 200000 => +50
	// 500000 => +100
	// 1000000 => +100
	salesPhaseWorkerCounts := []int{15, 5, 5, 5, 20, 50, 100, 100}

	// Calculate total workers needed
	totalTicketWorkers := 0
	for _, count := range ticketPhaseWorkerCounts {
		totalTicketWorkers += count
	}
	totalSalesWorkers := 0
	for _, count := range salesPhaseWorkerCounts {
		totalSalesWorkers += count
	}
	totalWorkers := totalTicketWorkers + totalSalesWorkers

	// Pre-spawn all workers as goroutines
	workerDone := make(chan struct{})
	var workersWg sync.WaitGroup
	workersWg.Add(totalWorkers)

	// Spawn ticket phase workers
	workerIdx := 0
	for phaseIdx, count := range ticketPhaseWorkerCounts {
		for i := 0; i < count; i++ {
			go func(phase int, phaseChan chan struct{}) {
				defer workersWg.Done()
				// Wait until this phase is active (blocks with zero CPU until channel is closed)
				select {
				case <-phaseChan:
					// Phase activated
				case <-ctx.Done():
					return
				}
				// Run user scenario in a loop until context is done
				for {
					select {
					case <-ctx.Done():
						return
					default:
						scenario.RunUserScenario(ctx)
					}
				}
			}(phaseIdx, ticketPhaseChans[phaseIdx])
			workerIdx++
		}
	}

	// Spawn sales phase workers
	for phaseIdx, count := range salesPhaseWorkerCounts {
		for i := 0; i < count; i++ {
			go func(phase int, phaseChan chan struct{}) {
				defer workersWg.Done()
				// Wait until this phase is active (blocks with zero CPU until channel is closed)
				select {
				case <-phaseChan:
					// Phase activated
				case <-ctx.Done():
					return
				}
				// Run user scenario in a loop until context is done
				for {
					select {
					case <-ctx.Done():
						return
					default:
						scenario.RunUserScenario(ctx)
					}
				}
			}(phaseIdx, salesPhaseChans[phaseIdx])
			workerIdx++
		}
	}

	// Function to activate phase flags
	var lastTicketPhase atomic.Int32
	lastTicketPhase.Store(-1)
	var lastSalesPhase atomic.Int32
	lastSalesPhase.Store(-1)

	addWorkersFn := func(newTicketPhase, newSalesPhase int32) {
		oldTicketPhase := lastTicketPhase.Load()
		oldSalesPhase := lastSalesPhase.Load()

		// Handle ticket phase change - close channels to activate workers (only once per channel)
		if newTicketPhase > oldTicketPhase {
			for phase := oldTicketPhase + 1; phase <= newTicketPhase; phase++ {
				p := phase // Capture for closure
				ticketPhaseOnce[p].Do(func() {
					close(ticketPhaseChans[p])
					addedWorkers := ticketPhaseWorkerCounts[p]
					currentTimeStr := getApplicationClock(scenario.initializedAt)
					log.Info("New ad campaign launched!",
						"ticket_phase", fmt.Sprintf("%d/%d", p, len(ticketSoldPhases)),
						"new_buyers", addedWorkers,
						"current_time", currentTimeStr,
						"user", "admin",
					)
				})
			}
			lastTicketPhase.Store(newTicketPhase)
		}

		// Handle sales phase change - close channels to activate workers (only once per channel)
		if newSalesPhase > oldSalesPhase {
			for phase := oldSalesPhase + 1; phase <= newSalesPhase; phase++ {
				p := phase // Capture for closure
				salesPhaseOnce[p].Do(func() {
					close(salesPhaseChans[p])
					addedWorkers := salesPhaseWorkerCounts[p]
					currentTimeStr := getApplicationClock(scenario.initializedAt)
					log.Info("New ad campaign launched!",
						"sales_phase", fmt.Sprintf("%d/%d", p, len(salesPhases)),
						"new_buyers", addedWorkers,
						"current_time", currentTimeStr,
						"user", "admin",
					)
				})
			}
			lastSalesPhase.Store(newSalesPhase)
		}
	}

	scenario.addWorkersFn = addWorkersFn

	// Activate initial phases (phase 0) by closing channels (using Once to prevent double-close)
	ticketPhaseOnce[0].Do(func() {
		close(ticketPhaseChans[0])
	})
	salesPhaseOnce[0].Do(func() {
		close(salesPhaseChans[0])
	})

	// Monitor for workers completion
	go func() {
		workersWg.Wait()
		close(workerDone)
	}()

	// Wait for either worker completion or critical error
	var criticalErrorMessage string
	select {
	case <-workerDone:
		// Normal completion
	case critErr := <-criticalError:
		// Critical error occurred, cancel context and stop benchmark
		criticalErrorMessage = critErr.Error()
		slog.Error("Critical error occurred, stopping benchmark", "error", criticalErrorMessage)
		cancel()
		// Wait a bit for goroutines to clean up
		<-workerDone
	}

	currentTimeStr = getApplicationClock(scenario.initializedAt)
	finalSales := sumShardedCounter(&totalSales)
	finalPurchased := sumShardedCounter(&totalPurchased)
	finalTickets := sumShardedCounter(&totalTickets)
	finalTicketPhase := currentTicketPhaseIndex.Load()
	finalSalesPhase := currentSalesPhaseIndex.Load()
	fmt.Printf("\nMain phase finished. Waiting for pending refunds to complete... (current_time: %s)\n", currentTimeStr)

	// Wait for all refund operations to complete with 10 second timeout (but don't treat timeout as error)
	refundDone := make(chan struct{})
	go func() {
		refundWg.Wait()
		close(refundDone)
	}()

	select {
	case <-refundDone:
		// Refunds completed successfully
	case <-time.After(10 * time.Second):
		// Timeout - just continue (not a critical error)
		// TODO: Fix this. For some reason, refunds sometimes hang indefinitely.
	}

	// Check if there was a critical error during refund phase
	select {
	case critErr := <-criticalError:
		criticalErrorMessage = critErr.Error()
		slog.Error("Critical error occurred, stopping benchmark", "error", criticalErrorMessage)
		cancel()
	default:
		// No critical error, proceed with final score
	}

	finalRefunds := sumShardedCounter(&totalRefunds)
	score := int64((float64(finalSales) + float64(finalPurchased-finalSales)*0.5 - float64(finalRefunds)) / 100)

	time.Sleep(3 * time.Second) // Wait for slog to flush

	// Validate no double booking per section
	// Convert "ScheduleID|Seat|FromTo" to individual sections "ScheduleID|Seat|AB", "ScheduleID|Seat|BC", etc.
	sectionReservations := make(map[string]bool)
	duplicateFound := false
	duplicateSchedule := ""
	duplicateSeat := ""
	duplicateSection := ""

	scenario.purchasedReservations.Range(func(key, value interface{}) bool {
		reservation := value.(string) // e.g., "E2123|A-3|AD"
		parts := splitReservation(reservation)
		scheduleID := parts[0]
		seat := parts[1]
		fromTo := parts[2]

		// Expand to individual sections
		sections := expandToSections(fromTo)
		for _, section := range sections {
			sectionKey := scheduleID + "|" + seat + "|" + section
			if sectionReservations[sectionKey] {
				slog.Error("Double booking detected!", "section_key", sectionKey, "original_reservation", reservation)
				duplicateFound = true
				duplicateSchedule = scheduleID
				duplicateSeat = seat
				duplicateSection = section
				return false // Stop iteration
			}
			sectionReservations[sectionKey] = true
		}
		return true
	})

	if criticalErrorMessage == "" && duplicateFound {
		slog.Error("Double booking validation failed!")
		criticalErrorMessage = fmt.Sprintf("Double booking detected: Schedule %s, Seat %s, Section %s", duplicateSchedule, duplicateSeat, duplicateSection)
		score = 0
	}

	// Always output final results regardless of log level
	fmt.Println("\nBenchmark Finished!")
	if criticalErrorMessage != "" {
		fmt.Println("  Interrupted due to critical error:")
		fmt.Printf("  %s\n\n", criticalErrorMessage)
	}

	fmt.Printf("  Score: %d\n", score)
	fmt.Printf("  Total Sales: %d\n", finalSales)
	fmt.Printf("  Total Purchased: %d\n", finalPurchased)
	fmt.Printf("  Total Refunds: %d\n", finalRefunds)
	fmt.Printf("  Net Revenue: %d\n", finalSales-finalRefunds)
	fmt.Printf("  Total Tickets: %d\n", finalTickets)
	fmt.Printf("  Ticket Phase: %d/%d\n", finalTicketPhase, len(ticketSoldPhases))
	fmt.Printf("  Sales Phase: %d/%d\n", finalSalesPhase, len(salesPhases))
	fmt.Printf("  Current Time: %s\n\n", currentTimeStr)

	postScore(score, scenario.appLanguage)
}

// splitReservation splits "ScheduleID|Seat|FromTo" into parts
func splitReservation(reservation string) []string {
	return strings.Split(reservation, "|")
}

// expandToSections expands "AD" to ["AB", "BC", "CD"]
func expandToSections(fromTo string) []string {
	if len(fromTo) != 2 {
		return []string{}
	}

	from := fromTo[0]
	to := fromTo[1]

	// Station order: A, B, C, D, E
	stations := "ABCDE"
	fromIdx := strings.IndexByte(stations, from)
	toIdx := strings.IndexByte(stations, to)

	if fromIdx == -1 || toIdx == -1 {
		return []string{}
	}

	// Handle both directions
	sections := []string{}
	if fromIdx < toIdx {
		// Forward direction (e.g., A->D becomes AB, BC, CD)
		for i := fromIdx; i < toIdx; i++ {
			sections = append(sections, string(stations[i])+string(stations[i+1]))
		}
	} else {
		// Reverse direction (e.g., D->A becomes DC, CB, BA)
		for i := fromIdx; i > toIdx; i-- {
			sections = append(sections, string(stations[i])+string(stations[i-1]))
		}
	}

	return sections
}

func postScore(score int64, appLanguage string) {
	apiURL := os.Getenv("BENCH_SCOREBOARD_APIGW_URL")
	teamName := os.Getenv("BENCH_TEAM_NAME")
	if apiURL == "" && teamName == "" {
		return
	}

	location, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		slog.Error("Failed to send score")
		slog.Error("Error loading location.", "error", err.Error())
		return
	}
	now := time.Now().In(location)
	timestamp := now.Format(time.RFC3339)

	data := map[string]interface{}{
		"team":      teamName,
		"score":     score,
		"timestamp": timestamp,
		"language":  appLanguage,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		slog.Error("Failed to send score")
		slog.Error("Error encoding JSON.", "error", err.Error())
		return
	}

	// Create the PUT request
	req, err := http.NewRequest("PUT", apiURL+"teams", bytes.NewBuffer(jsonData))
	if err != nil {
		slog.Error("Failed to send score")
		slog.Error("Error creating request.", "error", err.Error())
		return
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")

	// Send the request using the http.DefaultClient
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("Failed to send score")
		slog.Error("Error sending request.", "error", err.Error())
		return
	}
	defer resp.Body.Close()
	slog.Info("Score sent to scoreboard")
}
