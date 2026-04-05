package main

import (
	"log"
	"net/http"
	"os"

	"meeting-booking/app/di"
)

// @title Meeting Room Booking API
// @version 1.1
// @description Service for room scheduling and slot booking.
// @host localhost:8080
// @BasePath /
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
func main() {
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	h := di.BuildHTTPHandler()
	log.Printf("meeting booking service is listening on %s", addr)
	if err := http.ListenAndServe(addr, h); err != nil {
		log.Fatal(err)
	}
}
