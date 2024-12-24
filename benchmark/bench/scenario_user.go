package bench

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"strconv"
	"time"

	"github.com/isucon/isucandar/agent"
	"github.com/isucon/isucandar/worker"
)

type User struct {
	Name               string
	Password           string
	GlobalPaymentToken string
	CreditAmount       int
}

type TrainAvailability struct {
	ArenaToBridge string `json:"Arena->Bridge"` // "lots", "few", "none"
	BridgeToCave  string `json:"Bridge->Cave"`
	CaveToDock    string `json:"Cave->Dock"`
	DockToEdge    string `json:"Dock->Edge"`
	EdgeToDock    string `json:"Edge->Dock"`
	DockToCave    string `json:"Dock->Cave"`
	CaveToBridge  string `json:"Cave->Bridge"`
	BridgeToArena string `json:"Bridge->Arena"`
}

type TrainDepartureAt struct {
	ArenaToBridge string `json:"Arena->Bridge"` // "HH:MM" format
	BridgeToCave  string `json:"Bridge->Cave"`
	CaveToDock    string `json:"Cave->Dock"`
	DockToEdge    string `json:"Dock->Edge"`
	EdgeToDock    string `json:"Edge->Dock"`
	DockToCave    string `json:"Dock->Cave"`
	CaveToBridge  string `json:"Cave->Bridge"`
	BridgeToArena string `json:"Bridge->Arena"`
}

type TrainSchedule struct {
	ID           string            `json:"id"`
	Availability TrainAvailability `json:"availability"`
	DepartureAt  TrainDepartureAt  `json:"departure_at"`
}

type TrainScheduleResp struct {
	Schedules []TrainSchedule `json:"schedules"`
}

// Travel plan.
type Itinerary struct {
	Stations       []string
	ArrivalTimes   []time.Time
	DepartureTimes []time.Time
}

var (
	stations = []string{"A", "B", "C", "D", "E"}
)

// type BoughtTicket struct {
//   entryToken string
//   departureAt string
// }

type LoginReq struct {
	Name     string `json:"name"`
	Password string `json:"password"`
}

type WaitingStatusResp struct {
	Status    string `json:"status"`
	NextCheck int    `json:"next_check"`
}

type SessionResp struct {
	Status    string `json:"status"`
	NextCheck int    `json:"next_check"`
}

type ReservationReq struct {
	ScheduleID    string `json:"schedule_id"`
	FromStationID string `json:"from_station_id"`
	ToStationID   string `json:"to_station_id"`
	NumPeople     int    `json:"num_people"`
}

type ReservationResp struct {
	Status    string       `json:"status"`
	Reserved  *Reservation `json:"reserved"`
	Recommend *Reservation `json:"recommend"`
	ErrorCode string       `json:"error_code"`
}

type Reservation struct {
	ReservationID string   `json:"reservation_id"`
	ScheduleID    string   `json:"schedule_id"`
	FromStation   string   `json:"from_station"`
	ToStation     string   `json:"to_station"`
	DepartureAt   string   `json:"departure_at"`
	Seats         []string `json:"seats"`
	TotalPrice    int      `json:"total_price"`
	IsDiscounted  bool     `json:"is_discounted"`
}

type PurchaseReq struct {
	ReservationID string `json:"reservation_id"`
}

type PurchaseResp struct {
	Status     string `json:"status"`
	EntryToken string `json:"entry_token"`
	QRCodeURL  string `json:"qr_code_url"`
}

func (s *Scenario) RunUserScenario(ctx context.Context) {
	agent, err := agent.NewAgent(agent.WithBaseURL(s.targetURL), agent.WithTimeout(10*time.Second), agent.WithDefaultTransport())
	if err != nil {
		s.log.Error("failed to create agent", err.Error())
	}

	user, err := s.getRandomUser()
	if err != nil {
		s.log.Error("failed to get random user", err.Error())
	}
	s.log.Info("START", "user", user.Name)

	s.postLogin(ctx, agent, user)

	s.waitInWaitingRoom(ctx, agent, user)

	childCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start worker to periodically GET `/api/schedules`
	scheduleWorker, err := worker.NewWorker(func(childCtx context.Context, _ int) {
		resp, err := HttpGet(childCtx, agent, "/api/schedules")
		if err != nil {
			s.log.Error("failed to get /api/schedules", err.Error(), "user", user.Name)
		}
		s.log.Debug("GET /api/schedules", "statusCode", resp.StatusCode, "user", user.Name)
		time.Sleep(1 * time.Second)
	}, worker.WithInfinityLoop(), worker.WithMaxParallelism(1))
	if err != nil {
		s.log.Error("failed to create GET /api/schedule worker", err.Error(), "user", user.Name)
	}
	go func() {
		scheduleWorker.Process(childCtx)
	}()

	// Start worker to buy tickets
	ticketScenarioWorker, err := worker.NewWorker(func(childCtx context.Context, _ int) {
		s.runBuyTicketScenario(childCtx, agent, user)
	}, worker.WithLoopCount(1), worker.WithMaxParallelism(1))
	if err != nil {
		s.log.Error("failed to create GET /api/schedule worker", err.Error(), "user", user.Name)
	}
	go func() {
		ticketScenarioWorker.Process(childCtx)
	}()

	// Finish if the session is expired
	s.checkSession(ctx, agent, user)

	s.log.Info("user", user.Name, "Session ended", "user", user.Name)
}

func (s *Scenario) makeReservation(ctx context.Context, agent *agent.Agent, user User, req ReservationReq) (*ReservationResp, error) {
	reqBodyBuf, err := json.Marshal(req)
	if err != nil {
		s.log.Error("failed to parse JSON", err.Error(), "user", user.Name)
		return nil, err
	}
	resp, err := HttpPost(ctx, agent, "/api/reserve", bytes.NewReader(reqBodyBuf))
	if err != nil {
		s.log.Error("failed to post /api/reserve", err.Error(), "user", user.Name)
		return nil, err
	}
	s.log.Info("POST /api/reserve", "statusCode", resp.StatusCode, "user", user.Name)

	var reservationResp ReservationResp
	if err := json.Unmarshal(resp.Body, &reservationResp); err != nil {
		s.log.Error("failed to unmarshal response", err.Error(), "user", user.Name)
		return nil, err
	}

	return &reservationResp, nil
}

func (s *Scenario) purchaseReservation(ctx context.Context, agent *agent.Agent, user User, req PurchaseReq) (*PurchaseResp, error) {
	reqBodyBuf, err := json.Marshal(req)
	if err != nil {
		s.log.Error("failed to parse JSON", err.Error(), "user", user.Name)
		return nil, err
	}
	resp, err := HttpPost(ctx, agent, "/api/purchase", bytes.NewReader(reqBodyBuf))
	if err != nil {
		s.log.Error("failed to post /api/purchase", err.Error(), "user", user.Name)
		return nil, err
	}
	s.log.Info("POST /api/purchase", "statusCode", resp.StatusCode, "user", user.Name)

	var purchaseResp PurchaseResp
	if err := json.Unmarshal(resp.Body, &purchaseResp); err != nil {
		return nil, err
	}

	return &purchaseResp, nil
}

func getApplicationClock(initializedAt time.Time) string {
	timePassedInSec := time.Now().Sub(initializedAt).Seconds()
	hours := math.Min(math.Floor(timePassedInSec/6), 24)
	if hours == 24 {
		return "24:00"
	}
	minutes := math.Mod(timePassedInSec, 6) * 10
	return fmt.Sprintf("%02d:%02d", hours, minutes)
}

func (s *Scenario) runBuyTicketScenario(ctx context.Context, agent *agent.Agent, user User) error {
	s.sendInitRequests(ctx, agent, user)

	resp, err := HttpGet(ctx, agent, "/api/schedules")
	if err != nil {
		s.log.Error("failed to get /api/schedules", err.Error(), "user", user.Name)
	}

	var schedules TrainScheduleResp
	if err := json.Unmarshal(resp.Body, &schedules); err != nil {
		return err
	}

	itinerary := generateRandomItinerary()
	s.log.Info("Generated itinerary", "stations", itinerary.Stations)

	currentTime := getApplicationClock(s.initializedAt)

	numPeople := decideNumPeople(user.CreditAmount, itinerary)

	for i := 0; i < len(itinerary.Stations)-1; i++ {
		from := itinerary.Stations[i]
		to := itinerary.Stations[i+1]

		// Find the earliest schedule for this leg after currentTime
		schedule, departureTimeStr, err := findEarliestSchedule(from, to, currentTime, schedules.Schedules)
		if err != nil {
			s.log.Warn("No available schedule found for leg", "from", from, "to", to, "currentTime", currentTime, "error", err.Error(), "user", user.Name)
			return err
		}

		s.log.Info("Attempting to reserve ticket", "from", from, "to", to, "departure_at", departureTimeStr, "schedule_id", schedule.ID, "numPeople", numPeople)

		// Make reservation request
		reservationReq := ReservationReq{
			ScheduleID:    schedule.ID,
			FromStationID: from,
			ToStationID:   to,
			NumPeople:     numPeople,
		}
		reservationResp, err := s.makeReservation(ctx, agent, user, reservationReq)
		if err != nil {
			s.log.Error("Reservation request failed", "from", from, "to", to, "schedule_id", schedule.ID, "numPeople", numPeople, "error", err.Error())
			return err
		}

		// Handle reservation response
		var reservation *Reservation
		if reservationResp.Status == "success" && reservationResp.Reserved != nil {
			reservation = reservationResp.Reserved
			s.log.Info("Reservation successful", "reservation_id", reservation.ReservationID)
		} else if reservationResp.Status == "recommend" && reservationResp.Recommend != nil {
			reservation = reservationResp.Recommend
			// Decide whether to proceed with recommendation
			decision := rand.Float64()
			if decision < 0.2 {
				s.log.Warn("Recommendation rejected with 20% probability, cancelling reservation", "recommendation_id", reservation.ReservationID)
				return nil
			}
			s.log.Info("Proceeding with recommended reservation", "reservation_id", reservation.ReservationID)
		} else {
			s.log.Error("Reservation failed with unknown status", "status", reservationResp.Status)
			return nil
		}

		// Purchase the reservation
		purchaseReq := PurchaseReq{
			ReservationID: reservation.ReservationID,
		}
		_, err = s.purchaseReservation(ctx, agent, user, purchaseReq)
		if err != nil {
			s.log.Error("Failed to purchase reservation", "reservation_id", reservation.ReservationID, "error", err.Error())
			return err
		}
		s.log.Info("Purchase successful", "reservation_id", reservation.ReservationID)

		// TODO: Run acync worker to entry. Pass qr_code_url and request it before entry.
		// Check time and if it's exceeded, request refund.
		// /api/entry
		// entry_token: "ABC"

		// Determine the next departure time
		// Since the trip time on the train never exceeds 2 hours, we can use the departure time 2-6 hours
		hoursPassed := rand.Intn(5) + 2

		departureTime, _ := time.Parse("15:04", reservation.DepartureAt)
		nextDepartureTime := departureTime.Add(time.Duration(hoursPassed) * time.Hour)
		currentTime = nextDepartureTime.Format("15:04")

		// Stop if the current time exceeds 24:00
		if currentTime >= "24:00" {
			break
		}

		// Update schedules
		resp, err := HttpGet(ctx, agent, "/api/schedules")
		if err != nil {
			s.log.Error("failed to get /api/schedules", err.Error(), "user", user.Name)
		}

		if err := json.Unmarshal(resp.Body, &schedules); err != nil {
			return err
		}
	}

	return nil
}

func (s *Scenario) sendInitRequests(ctx context.Context, agent *agent.Agent, user User) {
	resp, err := HttpGet(ctx, agent, "/api/purchased_tickets")
	if err != nil {
		s.log.Error("failed to get /api/purchased_tickets", err.Error(), "user", user.Name)
	}
	s.log.Info("GET /api/purchased_tickets", "statusCode", resp.StatusCode, "user", user.Name)

	resp, err = HttpGet(ctx, agent, "/api/stations")
	if err != nil {
		s.log.Error("failed to get /api/stations", err.Error(), "user", user.Name)
	}
	s.log.Info("GET /api/stations", "statusCode", resp.StatusCode, "user", user.Name)

	resp, err = HttpGet(ctx, agent, "/api/current_time")
	if err != nil {
		s.log.Error("failed to get /api/current_time", err.Error(), "user", user.Name)
	}
	s.log.Info("GET /api/current_time", "statusCode", resp.StatusCode, "user", user.Name)
}

func (s *Scenario) postLogin(ctx context.Context, agent *agent.Agent, user User) error {
	s.log.Debug("POST /api/login", "user", user.Name)
	reqBody := &LoginReq{
		Name:     user.Name,
		Password: user.Password,
	}
	reqBodyBuf, err := json.Marshal(reqBody)
	if err != nil {
		s.log.Error("failed to parse JSON", err.Error(), "user", user.Name)
		return err
	}
	resp, err := HttpPost(ctx, agent, "/api/login", bytes.NewReader(reqBodyBuf))
	if err != nil {
		s.log.Error("failed to post /api/login", err.Error(), "user", user.Name)
		return err
	}
	s.log.Info("POST /api/login", "statusCode", resp.StatusCode, "user", user.Name)
	return nil
}

func (s *Scenario) waitInWaitingRoom(ctx context.Context, agent *agent.Agent, user User) error {
	for {
		resp, err := HttpGet(ctx, agent, "/api/waiting_status")
		if err != nil {
			return err
		}

		var waitingStatus WaitingStatusResp
		if err := json.Unmarshal(resp.Body, &waitingStatus); err != nil {
			return err
		}

		s.log.Info("GET /api/waiting_status", "status", waitingStatus.Status, "next_check", waitingStatus.NextCheck, "user", user.Name)

		if waitingStatus.Status == "ready" {
			break
		} else if waitingStatus.Status == "waiting" {
			time.Sleep(time.Duration(waitingStatus.NextCheck) * time.Millisecond)
		} else {
			s.log.Error("Unknown status", waitingStatus.Status, "Stopping requests.", "user", user.Name)
			break
		}
	}
	return nil
}

func (s *Scenario) checkSession(ctx context.Context, agent *agent.Agent, user User) error {
	for {
		resp, err := HttpGet(ctx, agent, "/api/session")
		if err != nil {
			return err
		}

		var session SessionResp
		if err := json.Unmarshal(resp.Body, &session); err != nil {
			return err
		}

		s.log.Info("GET /api/session", "status", session.Status, "next_check", session.NextCheck, "user", user.Name)

		// Check status
		if session.Status == "session_expired" {
			s.log.Info("Session expired. Stopping requests.", "user", user.Name)
			break
		} else if session.Status == "active" {
			// Wait next_check milliseconds before next request
			time.Sleep(time.Duration(session.NextCheck) * time.Millisecond)
		} else {
			// Unknown status, decide what to do:
			s.log.Info("Unknown status", session.Status, "Stopping requests.", "user", user.Name)
			break
		}
	}
	return nil
}

func (s *Scenario) getRandomUser() (User, error) {
	csvFilePath := "./bench/data/users.csv"
	userTotalCount := 1001

	file, err := os.Open(csvFilePath)
	if err != nil {
		return User{}, fmt.Errorf("failed to open CSV file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.TrimLeadingSpace = true

	// Read the header line
	headers, err := reader.Read()
	if err != nil {
		return User{}, fmt.Errorf("failed to read CSV headers: %w", err)
	}

	// Map headers to their indices for flexibility
	headerMap := make(map[string]int)
	for idx, header := range headers {
		headerMap[header] = idx
	}

	// Ensure required headers are present
	requiredHeaders := []string{"name", "password", "global_payment_token", "credit_amount"}
	for _, header := range requiredHeaders {
		if _, exists := headerMap[header]; !exists {
			return User{}, fmt.Errorf("missing required header: %s", header)
		}
	}

	var selectedUser User
	var count int = 0
	index := rand.Intn(userTotalCount)

	for {
		record, err := reader.Read()
		if err != nil {
			if errors.Is(err, os.ErrClosed) || errors.Is(err, io.EOF) {
				break
			}
			return User{}, fmt.Errorf("error reading CSV record: %w", err)
		}

		count++

		if count == index {
			creditAmount, _ := strconv.Atoi(record[headerMap["credit_amount"]])
			selectedUser = User{
				Name:               record[headerMap["name"]],
				Password:           record[headerMap["password"]],
				GlobalPaymentToken: record[headerMap["global_payment_token"]],
				CreditAmount:       creditAmount,
			}
			break
		}
	}

	return selectedUser, nil
}

func findEarliestSchedule(from string, to string, after string, schedules []TrainSchedule) (*TrainSchedule, string, error) {
	var earliestSchedule *TrainSchedule
	departureTime := "24:00" // Initialize with the slowest time

	for _, schedule := range schedules {
		// Get the departure time string for this leg
		scheduleDepartureStr := getScheduleDeparture(schedule.DepartureAt, from, to)
		if scheduleDepartureStr == "" {
			return nil, "", fmt.Errorf("no departure time found for %s -> %s", from, to)
		}

		if scheduleDepartureStr < after {
			continue
		}

		if !getScheduleAvailability(schedule.Availability, from, to) {
			continue
		}

		if scheduleDepartureStr < departureTime {
			earliestSchedule = &schedule
			departureTime = scheduleDepartureStr
		}
	}

	if earliestSchedule == nil {
		return nil, "", fmt.Errorf("no available schedule found for %s -> %s after %s", from, to, after)
	}

	// TODO: add some sleep to check if the application have enough session timeout

	return earliestSchedule, departureTime, nil
}

func indexOf(slice []string, target string) int {
	for i, v := range slice {
		if v == target {
			return i
		}
	}
	return -1
}

func reverseSlice(s []string) []string {
	n := len(s)
	reversed := make([]string, n)
	for i, v := range s {
		reversed[n-1-i] = v
	}
	return reversed
}

func getScheduleDeparture(departureAt TrainDepartureAt, from string, to string) string {
	var nextStation string
	idx := indexOf(stations, from)
	if from < to {
		nextStation = stations[idx+1]
	} else {
		nextStation = stations[idx-1]
	}
	key := fmt.Sprintf("%s->%s", from, nextStation)

	switch key {
	case "A->B":
		return departureAt.ArenaToBridge
	case "B->C":
		return departureAt.BridgeToCave
	case "C->D":
		return departureAt.CaveToDock
	case "D->E":
		return departureAt.DockToEdge
	case "E->D":
		return departureAt.EdgeToDock
	case "D->C":
		return departureAt.DockToCave
	case "C->B":
		return departureAt.CaveToBridge
	case "B->A":
		return departureAt.BridgeToArena
	default:
		return ""
	}
}

func getScheduleAvailability(availability TrainAvailability, from string, to string) bool {
	var stationsBetween []string
	fromIdx := indexOf(stations, from)
	toIdx := indexOf(stations, to)
	if from < to {
		stationsBetween = stations[fromIdx : toIdx+1]
	} else {
		stationsBetween = stations[toIdx : fromIdx+1]
		stationsBetween = reverseSlice(stationsBetween)
	}

	var a string
	for i := 0; i < len(stationsBetween)-1; i++ {
		from := stationsBetween[i]
		to := stationsBetween[i+1]
		key := fmt.Sprintf("%s->%s", from, to)
		switch key {
		case "A->B":
			a = availability.ArenaToBridge
		case "B->C":
			a = availability.BridgeToCave
		case "C->D":
			a = availability.CaveToDock
		case "D->E":
			a = availability.DockToEdge
		case "E->D":
			a = availability.EdgeToDock
		case "D->C":
			a = availability.DockToCave
		case "C->B":
			a = availability.CaveToBridge
		case "B->A":
			a = availability.BridgeToArena
		default:
			a = "none"
		}
		if a == "none" {
			return false
		}
	}
	return true
}

func generateRandomItinerary() *Itinerary {
	minStations := 2
	maxStations := 5
	numStations := rand.Intn(maxStations-minStations+1) + minStations

	itinerary := &Itinerary{
		Stations:       make([]string, 0, numStations),
		ArrivalTimes:   make([]time.Time, 0, numStations),
		DepartureTimes: make([]time.Time, 0, numStations),
	}

	currentStation := stations[rand.Intn(len(stations))]
	itinerary.Stations = append(itinerary.Stations, currentStation)

	for {
		nextStations := stations[rand.Intn(len(stations))]

		if currentStation == nextStations {
			continue
		}
		itinerary.Stations = append(itinerary.Stations, nextStations)
		currentStation = nextStations

		if len(itinerary.Stations) == numStations {
			break
		}
	}

	return itinerary
}

func decideNumPeople(creditAmount int, itinerary *Itinerary) int {
	totalDistance := 0
	baseTicketPrice := 1000
	minPeople := 1
	maxPeople := 50
	for i := 0; i < len(itinerary.Stations)-1; i++ {
		from := itinerary.Stations[i]
		to := itinerary.Stations[i+1]
		totalDistance += int(math.Abs(float64(indexOf(stations, from) - indexOf(stations, to))))
	}

	costPerPerson := baseTicketPrice * totalDistance
	// 90% of probability to choose lower than the maximum number of people
	// 10% of probability to choose more than the maximum number of people
	maxNumPeople := int(math.Ceil(float64(creditAmount) / float64(costPerPerson)))
	if rand.Float64() < 0.9 {
		if maxNumPeople < 15 {
			// 与信の付与額上、15人以下が結構多いので、ランダムを取って期待値を半分に圧縮する
			return rand.Intn(maxNumPeople) + 1
		} else if maxNumPeople < minPeople {
			return minPeople
		} else if maxNumPeople > maxPeople {
			return maxPeople
		}
	}
	return int(math.Min(float64(maxNumPeople+1), float64(maxPeople)))
}
