package bench

import (
	"encoding/json"
	"log/slog"
	"math/rand"
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
	targetURL     string
	initializedAt time.Time
	log           logger.Logger
	totalSales    *atomic.Int64
	totalRefunds  *atomic.Int64
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

	log := logger.GetLogger(logLevel)
	scenario := Scenario{
		targetURL:     targetURL,
		initializedAt: initResp.InitializedAt,
		log:           log,
		totalSales:    &totalSales,
		totalRefunds:  &totalRefunds,
	}

	currentTimeStr := getApplicationClock(scenario.initializedAt)
	slog.Info("Benchmark Start!", "current_time", currentTimeStr)

	worker, err := worker.NewWorker(func(ctx context.Context, _ int) {
		scenario.RunUserScenario(ctx)
	}, worker.WithMaxParallelism(8))
	if err != nil {
		panic(err)
	}
	worker.Process(ctx)

	currentTimeStr = getApplicationClock(scenario.initializedAt)
	finalSales := totalSales.Load()
	finalRefunds := totalRefunds.Load()
	slog.Info("Benchmark Finished!", "score", int64((finalSales-finalRefunds)/100), "total_sales", finalSales, "total_refunds", finalRefunds, "net_revenue", finalSales-finalRefunds, "current_time", currentTimeStr)
}
