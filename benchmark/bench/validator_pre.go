package bench

import (
	"context"
	"time"

	"github.com/isucon/isucandar/agent"
	"github.com/isucon/isucandar/worker"
)

func (s *Scenario) runPreValidation(ctx context.Context) error {
	agent, err := agent.NewAgent(agent.WithBaseURL(s.targetURL), agent.WithTimeout(10*time.Second), agent.WithDefaultTransport())
	if err != nil {
		s.log.Error("failed to create agent", err.Error())
	}

	user, err := s.getRandomUser(true)
	if err != nil {
		s.log.Error("failed to get random user for validation", err.Error())
	}
	s.log.Info("START PreValidation", "user", user.Name)

	s.postLogin(ctx, agent, user)
	s.waitInWaitingRoom(ctx, agent, user)

	// Start worker to buy tickets
	childCtx := context.Background()
	ticketScenarioWorker, err := worker.NewWorker(func(childCtx context.Context, _ int) {
		s.runBuyTicketValidation(childCtx, agent, user)
	}, worker.WithLoopCount(1), worker.WithMaxParallelism(1))
	if err != nil {
		s.log.Error("failed to create runBuyTicketValidation worker", err.Error(), "user", user.Name)
	}
	go func() {
		ticketScenarioWorker.Process(childCtx)
	}()

	currentTime := getApplicationClock(s.initializedAt)
	s.log.Info("PreValidation ended", "current time", currentTime, "user", user.Name)
	return nil
}

func (s *Scenario) runBuyTicketValidation(ctx context.Context, agent *agent.Agent, user User) error {
	// Buy the earliest ticket two times alone, 1) enter before departure, 2) enter after departure and refund.
	// and buy another ticket to check the credit limit exceeding.
	// Since MIN_CREDIT = 5000, we can at least by 1000, 3000 (for three person) yen tickets.

	// Check the displayed price is correct

	// Check entry token in QR code image

	// Try to enter twice using the same entry token

	// Check bought ticket list. Refunded tickets should not be in the list.
	return nil
}
