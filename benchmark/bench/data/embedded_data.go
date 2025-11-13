package data

import (
	_ "embed"
)

// Embedded CSV files for AWS Lambda compatibility
// These files are embedded at compile time, so they're always available
// regardless of the filesystem structure

//go:embed users.csv
var UsersCSV string

//go:embed train_configs_ticket_sold.csv
var TrainConfigsTicketSoldCSV string

//go:embed train_configs_sales.csv
var TrainConfigsSalesCSV string
