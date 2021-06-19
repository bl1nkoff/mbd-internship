package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"regexp"
	"time"

	"github.com/golang/geo/s2"
	"github.com/syndtr/goleveldb/leveldb"
)

/*var disp dispatcher.Dispatcher

type myJob struct {
	w            http.ResponseWriter
	dataJob      DataRequest
	collectorJob Signal
}

func (job *myJob) Do() {
	collectorHandler(job.w, job.collectorJob)
}*/

type Signal struct {
	Lat, Lng, Signal float64
	User_id          string
}

type Cell struct {
	Center      Coordinate
	Data        []Signal
	Coordinates []Coordinate
}

type Coordinate struct {
	Lat, Lon float64
}

type Data_reply struct {
	S2_id          uint64
	S2_coordinates []Coordinate
	Uniq_users     uint64
	Signal_avg     float64
}

type DataRequest struct {
	Area [2]Coordinate
}

func main() {
	var (
		maxWorkers   = flag.Int("max_workers", 5, "The number of workers to start")
		maxQueueSize = flag.Int("max_queue_size", 100, "The size of job queue")
		port         = flag.String("port", "8080", "The server port")
	)
	flag.Parse()

	// Create the job queue.
	jobQueue := make(chan Job, *maxQueueSize)

	// Start the dispatcher.
	dispatcher := NewDispatcher(jobQueue, *maxWorkers)
	dispatcher.run()

	// Start the HTTP handler.
	http.HandleFunc("/map", func(w http.ResponseWriter, r *http.Request) {
		mapHandler(w)
	})
	http.HandleFunc("/data", func(w http.ResponseWriter, r *http.Request) {
		dataHandler(w, r)
	})
	http.HandleFunc("/collector", func(w http.ResponseWriter, r *http.Request) {
		collectorHandler(w, r, jobQueue)
	})
	log.Fatal(http.ListenAndServe(":"+*port, nil))
}

func dataHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(500)
		return
	}
	var req DataRequest
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&req)
	if err != nil {
		w.WriteHeader(500)
		return
	}

	//verification
	for i := 0; i < 2; i++ {
		if req.Area[i].Lat < -90 || req.Area[i].Lat > 90 ||
			req.Area[i].Lon < -180 || req.Area[i].Lon > 180 {
			w.WriteHeader(500)
			return
		}
	}

	CoockDataBase()

	db, err := leveldb.OpenFile("datebase.db", nil)
	if err != nil {
		fmt.Println("рфывш")
		return
	} else {
		defer db.Close()
	}

	res := []Data_reply{}

	iter := db.NewIterator(nil, nil)
	for iter.Next() {
		value := bytesToCell(iter.Value())
		if value.Center.Lat > req.Area[0].Lat && value.Center.Lat < req.Area[1].Lat &&
			value.Center.Lon > req.Area[0].Lon && value.Center.Lon < req.Area[1].Lon {
			signals := value.Data
			summ := float64(0)
			uniq := []string{}
			for _, s := range signals {
				summ += s.Signal
				uniq = append(uniq, s.User_id)
			}
			cell := Data_reply{
				S2_id:          bytesToUint64(iter.Key()),
				S2_coordinates: value.Coordinates,
				Uniq_users:     uint64(len(unique(uniq))),
				Signal_avg:     summ / float64(len(signals)),
			}
			res = append(res, cell)
		}
	}

	iter.Release()

	if len(res) == 0 {
		w.WriteHeader(404)
		return
	}

	//form response
	w.Header().Set("Content-Type", "application/json")
	resBytes := new(bytes.Buffer)
	_ = json.NewEncoder(resBytes).Encode(&res)
	w.Write(resBytes.Bytes())
}

func mapHandler(w http.ResponseWriter) {
	parsedTemplate, _ := template.ParseFiles("html/map.html")
	err := parsedTemplate.Execute(w, nil)
	if err != nil {
		w.WriteHeader(500)
	}
}

func collectorHandler(w http.ResponseWriter, r *http.Request, jobQueue chan Job) {
	var s Signal
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&s)
	if err != nil {
		w.WriteHeader(500)
		return
	}
	/*disp.Dispatch(&myJob{
		w:            w,
		collectorJob: s,
	})*/

	//verification
	if s.Lat < -90 || s.Lat > 90 || s.Lng < -180 || s.Lng > 180 ||
		s.Signal < 0 || s.Signal > 100 || !IsValidUUID(s.User_id) {
		w.WriteHeader(500)
		return
	}

	db, _ := leveldb.OpenFile("datebaseRaw.db", nil)
	defer db.Close()

	_ = db.Put(TimetoBytes(), SignaltoBytes(s), nil)
}

func CoockDataBase() {
	dbR, _ := leveldb.OpenFile("datebaseRaw.db", nil)
	defer dbR.Close()
	db, _ := leveldb.OpenFile("datebase.db", nil)
	defer db.Close()

	iter := dbR.NewIterator(nil, nil)
	for iter.Next() {
		s := bytesToSignal(iter.Value())
		//S2
		ll := s2.LatLngFromDegrees(s.Lat, s.Lng)     //get LatLon for CellID
		cellID := s2.CellIDFromLatLng(ll).Parent(15) // get CellID, set 15 ур
		cell := s2.CellFromCellID(cellID)            // get Cell по

		//search in db
		cellKey := s2CellIDtoBytes(cellID)
		data, err := db.Get(cellKey, nil)

		if err == leveldb.ErrNotFound {

			//Cel was no found - create
			vertices := s2.PolygonFromCell(cell).Loop(0).Vertices()

			//Coordinates from Cell
			var coordinates []Coordinate
			for i := 0; i < len(vertices); i++ {
				latlng := s2.LatLngFromPoint(vertices[i])
				coordinate := Coordinate{latlng.Lat.Degrees(), latlng.Lng.Degrees()}
				coordinates = append(coordinates, coordinate)
			}

			//New Cell
			center := cell.RectBound().Center()
			newCell := Cell{
				Center:      Coordinate{center.Lat.Degrees(), center.Lng.Degrees()},
				Data:        []Signal{s},
				Coordinates: coordinates,
			}

			data = CelltoBytes(newCell)

		} else if err != nil {

			continue

		} else {

			//Cell exists
			oldCell := bytesToCell(data)
			oldCell.Data = append(oldCell.Data, s)
			data = CelltoBytes(oldCell)
		}

		//Write new cell in DB and Return status code
		_ = db.Put(cellKey, data, nil)
		err = dbR.Delete(iter.Key(), nil)
	}
	iter.Release()
}

func IsValidUUID(uuid string) bool {
	r := regexp.MustCompile("^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$")
	return r.MatchString(uuid)
}

func unique(stringSlice []string) []string {
	keys := make(map[string]bool)
	list := []string{}
	for _, entry := range stringSlice {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}

func s2CellIDtoBytes(value s2.CellID) []byte {
	valueBytes := new(bytes.Buffer)
	_ = json.NewEncoder(valueBytes).Encode(&value)
	return valueBytes.Bytes()
}

func CelltoBytes(value Cell) []byte {
	valueBytes := new(bytes.Buffer)
	_ = json.NewEncoder(valueBytes).Encode(&value)
	return valueBytes.Bytes()
}

func SignaltoBytes(value Signal) []byte {
	valueBytes := new(bytes.Buffer)
	_ = json.NewEncoder(valueBytes).Encode(&value)
	return valueBytes.Bytes()
}

func TimetoBytes() []byte {
	value := time.Now().UnixNano() / int64(time.Millisecond)
	valueBytes := new(bytes.Buffer)
	_ = json.NewEncoder(valueBytes).Encode(&value)
	return valueBytes.Bytes()
}

func bytesToCell(value []byte) Cell {
	result := Cell{}
	reqBodyBytes := bytes.NewBuffer(value)
	json.NewDecoder(reqBodyBytes).Decode(&result)
	return result
}

func bytesToUint64(value []byte) uint64 {
	var result uint64
	reqBodyBytes := bytes.NewBuffer(value)
	json.NewDecoder(reqBodyBytes).Decode(&result)
	return result
}

func bytesToSignal(value []byte) Signal {
	var result Signal
	reqBodyBytes := bytes.NewBuffer(value)
	json.NewDecoder(reqBodyBytes).Decode(&result)
	return result
}
