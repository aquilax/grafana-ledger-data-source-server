package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/howeyc/ledger"
)

const accountSeparator = ":"

type SearchRequest struct {
	Target string `json:"target"`
}

type QueryRequest struct {
	PanelID int `json:"panelId"`
	Range   struct {
		From time.Time `json:"from"`
		To   time.Time `json:"to"`
		Raw  struct {
			From string `json:"from"`
			To   string `json:"to"`
		} `json:"raw"`
	} `json:"range"`
	RangeRaw struct {
		From string `json:"from"`
		To   string `json:"to"`
	} `json:"rangeRaw"`
	Interval   string `json:"interval"`
	IntervalMs int    `json:"intervalMs"`
	Targets    []struct {
		Target string `json:"target"`
		RefID  string `json:"refId"`
		Type   string `json:"type"`
	} `json:"targets"`
	AdhocFilters []struct {
		Key      string `json:"key"`
		Operator string `json:"operator"`
		Value    string `json:"value"`
	} `json:"adhocFilters"`
	Format        string `json:"format"`
	MaxDataPoints int    `json:"maxDataPoints"`
}

type DataPoint struct {
	At    time.Time
	Value float64
}

func (dp DataPoint) MarshalJSON() ([]byte, error) {
	return json.Marshal([]interface{}{dp.Value, dp.At.Unix() * 1000})
}

type QueryResponse struct {
	Target     string      `json:"target"`
	DataPoints []DataPoint `json:"datapoints"`
}

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
		panic(err)
	}

	generalLedger, parseError := ledger.ParseLedger(ledgerFileReader)
	if parseError != nil {
		panic(parseError.Error())
	}

	getNames := func(name string) []string {
		result := []string{}
		buff := []string{}
		segments := strings.Split(name, accountSeparator)

		for i := 0; i < len(segments)-1; i++ {
			buff = append(buff, segments[i])
			// suffix partial matches with account separator
			result = append(result, strings.Join(buff, accountSeparator)+accountSeparator)
		}
		result = append(result, name)
		return result
	}

	// Populate search result
	t := make(map[string]struct{})
	for idx := 0; idx < len(generalLedger); idx++ {
		for i := 0; i < len(generalLedger[idx].AccountChanges); i++ {
			for _, n := range getNames(generalLedger[idx].AccountChanges[i].Name) {
				t[n] = struct{}{}
			}
		}
	}
	searchResult := make([]string, len(t))
	i := 0
	for key := range t {
		searchResult[i] = key
		i++
	}

	getTargetData := func(target string, from, to time.Time) []DataPoint {
		var dp []DataPoint
		var v float64
		var agg float64
		var hit bool
		for idx := 0; idx < len(generalLedger); idx++ {
			agg = 0
			hit = false
			for i := 0; i < len(generalLedger[idx].AccountChanges); i++ {
				if strings.HasPrefix(generalLedger[idx].AccountChanges[i].Name, target) && generalLedger[idx].Date.After(from) && generalLedger[idx].Date.Before(to) {
					v, _ = generalLedger[idx].AccountChanges[i].Balance.Float64()
					agg += v
					hit = true
				}
			}
			if hit {
				dp = append(dp, DataPoint{
					generalLedger[idx].Date,
					agg,
				})
			}
		}
		return dp
	}

	getQueryResponse := func(qr QueryRequest) ([]QueryResponse, error) {
		resp := make([]QueryResponse, len(qr.Targets))
		for i, t := range qr.Targets {
			resp[i] = QueryResponse{
				t.Target,
				getTargetData(t.Target, qr.Range.From, qr.Range.To),
			}
		}
		return resp, nil
	}

	// / should return 200 ok. Used for "Test connection" on the datasource config page.
	okHandler := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}

	// /search used by the find metric options on the query tab in panels.
	searchHandler := func(w http.ResponseWriter, r *http.Request) {
		j, _ := json.Marshal(searchResult)
		w.Write(j)
	}

	// /query should return metrics based on input.
	queryHandler := func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)
		var qr QueryRequest
		err := decoder.Decode(&qr)
		if err != nil {
			panic(err)
		}
		resp, _ := getQueryResponse(qr)
		j, err := json.Marshal(resp)
		if err != nil {
			panic(err)
		}
		w.Write(j)
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

	withCORS := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
			if (*r).Method == "OPTIONS" {
				return
			}
			next.ServeHTTP(w, r)
		}
	}

	withLogger := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			next.ServeHTTP(w, r)

			addr := r.RemoteAddr
			log.Printf("(%s) \"%s %s %s\" %s\n", addr, r.Method, r.RequestURI, r.Proto, time.Since(start))
		}
	}

	for _, c := range []struct {
		route   string
		handler http.HandlerFunc
	}{
		{"/", okHandler},
		{"/search", searchHandler},
		{"/query", queryHandler},
		{"/annotations", annotationsHandler},
		{"/tag-keys", tagKeysHandler},
		{"/tag-values", tagValuesHandler},
	} {
		http.HandleFunc(c.route, withLogger(withCORS(c.handler)))
	}

	log.Fatal(http.ListenAndServe(":8080", nil))
}
