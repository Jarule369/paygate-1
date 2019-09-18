// Copyright 2018 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package internal

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	moovhttp "github.com/moov-io/base/http"

	"github.com/go-kit/kit/log"
	"github.com/gorilla/mux"
)

type EventID string

type Event struct {
	ID      EventID   `json:"id"`
	Topic   string    `json:"topic"`
	Message string    `json:"message"`
	Type    EventType `json:"type"`

	// TODO(adam): We might need to inspect/filter events by metadata
	// map[string]string "transferId" -> "..."
}

type EventType string

const (
	// TODO(adam): more EventType values?
	// ReceiverEvent   EventType = "Receiver"
	// DepositoryEvent EventType = "Depository"
	// OriginatorEvent EventType = "Originator"
	TransferEvent EventType = "Transfer"
)

func AddEventRoutes(logger log.Logger, r *mux.Router, eventRepo EventRepository) {
	r.Methods("GET").Path("/events").HandlerFunc(getUserEvents(logger, eventRepo))
	r.Methods("GET").Path("/events/{eventID}").HandlerFunc(getEventHandler(logger, eventRepo))
}

func getUserEvents(logger log.Logger, eventRepo EventRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w, err := wrapResponseWriter(logger, w, r)
		if err != nil {
			return
		}

		userID := moovhttp.GetUserID(r)
		events, err := eventRepo.getUserEvents(userID)
		if err != nil {
			moovhttp.Problem(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(events)
	}
}

func getEventHandler(logger log.Logger, eventRepo EventRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w, err := wrapResponseWriter(logger, w, r)
		if err != nil {
			return
		}

		eventID, userID := getEventID(r), moovhttp.GetUserID(r)
		if eventID == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// grab event
		event, err := eventRepo.getEvent(eventID, userID)
		if err != nil {
			moovhttp.Problem(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(event)
	}
}

// getEventID extracts the EventID from the incoming request.
func getEventID(r *http.Request) EventID {
	v := mux.Vars(r)
	id, ok := v["eventID"]
	if !ok {
		return EventID("")
	}
	return EventID(id)
}

type EventRepository interface {
	getEvent(eventID EventID, userID string) (*Event, error)
	getUserEvents(userID string) ([]*Event, error)

	writeEvent(userID string, event *Event) error

	getUserTransferEvents(userID string, transferID TransferID) ([]*Event, error)
}

func NewEventRepo(logger log.Logger, db *sql.DB) *SQLEventRepo {
	return &SQLEventRepo{log: logger, db: db}
}

type SQLEventRepo struct {
	db  *sql.DB
	log log.Logger
}

func (r *SQLEventRepo) Close() error {
	return r.db.Close()
}

func (r *SQLEventRepo) writeEvent(userID string, event *Event) error {
	query := `insert into events (event_id, user_id, topic, message, type, created_at) values (?, ?, ?, ?, ?, ?)`
	stmt, err := r.db.Prepare(query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(event.ID, userID, event.Topic, event.Message, event.Type, time.Now())
	if err != nil {
		return err
	}
	return nil
}

func (r *SQLEventRepo) getEvent(eventID EventID, userID string) (*Event, error) {
	query := `select event_id, topic, message, type from events
where event_id = ? and user_id = ?
limit 1`
	stmt, err := r.db.Prepare(query)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	row := stmt.QueryRow(eventID, userID)

	event := &Event{}
	err = row.Scan(&event.ID, &event.Topic, &event.Message, &event.Type)
	if err != nil {
		return nil, err
	}
	if event.ID == "" {
		return nil, nil // event not found
	}
	return event, nil
}

func (r *SQLEventRepo) getUserEvents(userID string) ([]*Event, error) {
	query := `select event_id from events where user_id = ?`
	stmt, err := r.db.Prepare(query)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	rows, err := stmt.Query(userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var eventIDs []string
	for rows.Next() {
		var row string
		if err := rows.Scan(&row); err != nil {
			return nil, fmt.Errorf("getUserEvents scan: %v", err)
		}
		if row != "" {
			eventIDs = append(eventIDs, row)
		}
	}
	var events []*Event
	for i := range eventIDs {
		event, err := r.getEvent(EventID(eventIDs[i]), userID)
		if err == nil && event != nil {
			events = append(events, event)
		}
	}
	return events, rows.Err()
}

func (r *SQLEventRepo) getUserTransferEvents(userID string, id TransferID) ([]*Event, error) {
	// TODO(adam): need to store transferID alongside in some arbitrary json
	// Scan on Type == TransferEvent ?
	return nil, nil
}
