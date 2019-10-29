package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/howeyc/ledger"
)

func main() {
	var ledgerFileName string

	flag.StringVar(&ledgerFileName, "f", "", "Ledger file name (*Required).")
	flag.Parse()

	if len(ledgerFileName) == 0 {
		flag.Usage()
		return
	}

	ledgerFileReader, err := ledger.NewLedgerReader(ledgerFileName)
	if err != nil {
		fmt.Println(err)
		return
	}

	generalLedger, parseError := ledger.ParseLedger(ledgerFileReader)
	if parseError != nil {
		fmt.Printf("%s\n", parseError.Error())
		return
	}

	// Populate search result
	t := make(map[string]bool)
	for idx := 0; idx < len(generalLedger); idx++ {
		for i := 0; i < len(generalLedger[idx].AccountChanges); i++ {
			t[generalLedger[idx].AccountChanges[i].Name] = true
		}
	}
	searchResult := make([]string, len(t))
	n := 0
	for key := range t {
		searchResult[n] = key
		n++
	}

	// / should return 200 ok. Used for "Test connection" on the datasource config page.
	okHandler := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}

	// /search used by the find metric options on the query tab in panels.
	searchHandler := func(w http.ResponseWriter, _ *http.Request) {
		j, _ := json.Marshal(searchResult)
		w.Write(j)
	}

	// /query should return metrics based on input.
	queryHandler := func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `[]`)
	}

	// /annotations should return annotations.
	annotationsHandler := func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("[]"))
	}

	tagKeysHandler := func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("[{\"type\":\"string\",\"text\":\"Account\"}]"))
	}

	tagValuesHandler := func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("[]"))
	}

	logger := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
			if (*r).Method == "OPTIONS" {
				return
			}

			next.ServeHTTP(w, r)

			addr := r.RemoteAddr

			fmt.Printf("(%s) \"%s %s %s\" %s\n", addr, r.Method, r.RequestURI, r.Proto, time.Since(start))
		}

	}

	http.HandleFunc("/", logger(okHandler))
	http.HandleFunc("/search", logger(searchHandler))
	http.HandleFunc("/query", logger(queryHandler))
	http.HandleFunc("/annotations", logger(annotationsHandler))
	http.HandleFunc("/tag-keys", logger(tagKeysHandler))
	http.HandleFunc("/tag-values", logger(tagValuesHandler))

	log.Fatal(http.ListenAndServe(":8080", nil))
}
