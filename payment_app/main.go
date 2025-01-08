package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

// userInfo stores a single user's data
type userInfo struct {
	Name               string
	Password           string
	GlobalPaymentToken string
	CreditAmount       int
}

// In-memory store for user data
var (
	userStore = make(map[string]*userInfo) // key: GlobalPaymentToken
	mu        sync.RWMutex                 // protects userStore
)

// loadCSV reads the CSV file and populates userStore
func loadCSV(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("unable to open CSV file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.Read() // skip header

	records, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("unable to read CSV file: %w", err)
	}

	// We acquire a write lock to safely modify the map
	mu.Lock()
	defer mu.Unlock()

	// Clear the current store
	userStore = make(map[string]*userInfo)

	for _, row := range records {
		// name, password, global_payment_token, credit_amount
		if len(row) < 4 {
			// skip or handle error
			continue
		}

		credit, err := strconv.Atoi(row[3])
		if err != nil {
			// skip or handle error
			continue
		}

		user := &userInfo{
			Name:               row[0],
			Password:           row[1],
			GlobalPaymentToken: row[2],
			CreditAmount:       credit,
		}
		userStore[user.GlobalPaymentToken] = user
	}

	return nil
}

type paymentRequest struct {
	GlobalPaymentToken string `json:"global_payment_token"`
	Amount             int    `json:"amount"`
}

type paymentResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// POST /payments
func handlePayments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req paymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Sleep for random duration between 1 and 2 seconds
	// To simulate payment network latency
	randomDuration := time.Duration(rand.Intn(1000)+1000) * time.Millisecond
	time.Sleep(randomDuration)

	mu.Lock()
	defer mu.Unlock()

	user, exists := userStore[req.GlobalPaymentToken]
	if !exists {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(paymentResponse{
			Status:  "error",
			Message: "user not found",
		})
		return
	}

	if user.CreditAmount < req.Amount {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(paymentResponse{
			Status:  "error",
			Message: "insufficient credit",
		})
		return
	}

	user.CreditAmount -= req.Amount

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(paymentResponse{
		Status:  "accepted",
		Message: "payment captured",
	})
}

// POST /initialize to refresh the user data
func handleInitialize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if err := loadCSV("users.csv"); err != nil {
		http.Error(w, "failed to load CSV", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "User data refreshed from CSV\n")
}

func main() {
	rand.New(rand.NewSource(time.Now().UnixNano())) // Seed random number generator

	// Load CSV on startup
	if err := loadCSV("users.csv"); err != nil {
		log.Fatalf("Failed to load CSV: %v", err)
	}

	http.HandleFunc("/payments", handlePayments)
	http.HandleFunc("/initialize", handleInitialize)
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK\n")
	})

	fmt.Println("Server running on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
