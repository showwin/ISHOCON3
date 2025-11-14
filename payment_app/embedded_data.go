package main

import (
	_ "embed"
)

// Embedded CSV file for AWS Lambda compatibility
// This file is embedded at compile time, so it's always available
// regardless of the filesystem structure

//go:embed users.csv
var UsersCSV string
