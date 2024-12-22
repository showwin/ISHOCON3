package bench

import (
	"os"
	"time"
	"log/slog"
  "math/rand"

	"golang.org/x/net/context"

  "github.com/showwin/ISHOCON3/benchmark/bench/logger"

  "github.com/isucon/isucandar/worker"
)

var jst = time.FixedZone("Asia/Tokyo", 9*60*60)

type contextKey string

const (
    targetURLKey contextKey = "targetURL"
)

type NewUserCount struct {
  Count int
}

type Scenario struct {
  log logger.Logger
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

func Run(targetURL string) {
  rand.Seed(time.Now().UnixNano())

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == "time" && a.Value.Kind() == slog.KindTime {
				return slog.String(a.Key, a.Value.Time().In(jst).Format("15:04:05.000"))
			}
			return a
		},
	})))

  ctx, cancel := context.WithTimeout(context.Background(), 60 * time.Second)
  defer cancel()

  ctx = context.WithValue(ctx, targetURLKey, targetURL)

  // TODO: maybe better to have more buffer size. Currently 10.
  // boughtSeat := make(chan BoughtSeat, 10)
  // score := make(chan Score, 10)

  log := logger.GetLogger()
  scenario := NewScenario(log)

  worker, err := worker.NewWorker(func(ctx context.Context, _ int) {
		// RunUserScenario(ctx, boughtSeat, score)
    scenario.RunUserScenario(ctx)
	}, worker.WithMaxParallelism(1))
	if err != nil {
		panic(err)
	}
  worker.Process(ctx)


  slog.Info("Benchmark Finished!")

	// getInitialize()
	// log.Print("Benchmark Start!  Workload: " + strconv.Itoa(workload))
	// finishTime := time.Now().Add(1 * time.Minute)
	// validateInitialize()
	// wg := new(sync.WaitGroup)
	// m := new(sync.Mutex)
	// for i := 0; i < workload; i++ {
	// 	wg.Add(1)
	// 	if i%3 == 0 {
	// 		go loopJustLookingScenario(wg, m, finishTime)
	// 	} else if i%3 == 1 {
	// 		go loopStalkerScenario(wg, m, finishTime)
	// 	} else {
	// 		go loopBakugaiScenario(wg, m, finishTime)
	// 	}
	// }
	// wg.Wait()
}



func NewScenario(log logger.Logger) *Scenario {
    return &Scenario{log: log}
}
