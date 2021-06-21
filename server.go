package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"regexp"
	"time"

	//"github.com/YSZhuoyang/go-dispatcher/dispatcher"
	"github.com/golang/geo/s2"
	"github.com/syndtr/goleveldb/leveldb"
)

var PORT = "8080"

//var disp dispatcher.Dispatcher

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

/*type myJob struct {
	Signal Signal
}

func (job *myJob) Do() {
	err := collectorDataBaseHandler(job.Signal)
	if err != nil {
		fmt.Println("Error")
	}
}*/

func main() {
	//disp, _ = dispatcher.NewDispatcher(1000)
	fmt.Println("Listening on " + PORT)
	http.HandleFunc("/map", func(w http.ResponseWriter, r *http.Request) {
		mapHandler(w)
	})
	http.HandleFunc("/data", func(w http.ResponseWriter, r *http.Request) {
		dataHandler(w, r)
	})
	http.HandleFunc("/collector", func(w http.ResponseWriter, r *http.Request) {
		collectorHandler(w, r)
	})
	http.Handle("/", http.FileServer(http.Dir("html/")))
	log.Fatal(http.ListenAndServe(":"+PORT, nil))
	//disp.Finalize()
}

func dataHandler(w http.ResponseWriter, r *http.Request) {
	//Разрешаем все источники
	w.Header().Set("Access-Control-Allow-Origin", "*")

	//Проверяем метод
	if r.Method != "POST" {
		w.WriteHeader(400)
		return
	}

	//Забираем данные из POST
	var req DataRequest
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&req)
	if err != nil {
		w.WriteHeader(400)
		return
	}

	//Проверяем данные
	for i := 0; i < 2; i++ {
		if req.Area[i].Lat < -90 || req.Area[i].Lat > 90 ||
			req.Area[i].Lon < -180 || req.Area[i].Lon > 180 {
			w.WriteHeader(400)
			return
		}
	}

	//"Готовим" "сырую" базу данных
	CoockDataBase()

	//Переходим к основной
	db, err := leveldb.OpenFile("database.db", nil)
	if err != nil {
		w.WriteHeader(500)
		return
	} else {
		defer db.Close()
	}

	//Создаём и заполняем массив для ответа
	res := []Data_reply{}

	iter := db.NewIterator(nil, nil)
	for iter.Next() {
		value := bytesToCell(iter.Value())
		//Попадает ли клетка в поле видимости
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

	//Если ничего не попадает, то возвращем код (без него js не хотел парсить "[]")
	if len(res) == 0 {
		w.WriteHeader(404)
		return
	}

	//Отдаём JSON
	w.Header().Set("Content-Type", "application/json")
	resBytes := new(bytes.Buffer)
	_ = json.NewEncoder(resBytes).Encode(&res)
	w.Write(resBytes.Bytes())
}

func mapHandler(w http.ResponseWriter) {
	//Отдаём html страницу
	parsedTemplate, err := template.ParseFiles("html/map.html")
	if err != nil {
		w.WriteHeader(500)
		return
	}
	err = parsedTemplate.Execute(w, nil)
	if err != nil {
		w.WriteHeader(500)
	}
}

func collectorHandler(w http.ResponseWriter, r *http.Request) {
	//Разрешаем все источники
	w.Header().Set("Access-Control-Allow-Origin", "*")

	//Проверяем метод
	if r.Method != "POST" {
		w.WriteHeader(400)
		return
	}
	//Забираем данные из POST
	var s Signal
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&s)
	if err != nil {
		w.WriteHeader(500)
		return
	}

	//Проверяем данные
	if s.Lat < -90 || s.Lat > 90 || s.Lng < -180 || s.Lng > 180 ||
		s.Signal < 0 || s.Signal > 100 || !IsValidUUID(s.User_id) {
		w.WriteHeader(400)
		return
	}

	/*disp.Dispatch(&myJob{
		Signal: s,
	})*/
	go collectorDataBaseHandler(s)
}

func collectorDataBaseHandler(s Signal) error {
	//Подключаемся к "сырой" базе данных
	db, _ := leveldb.OpenFile("databaseRaw.db", nil)
	defer db.Close()

	//Отправляем данные туда. В виде ключа используем время в милисекундах, просто как уникальную строку
	return db.Put(TimetoBytes(), SignaltoBytes(s), nil)
}

func CoockDataBase() {
	//Подключаемся к базам
	dbR, err := leveldb.OpenFile("databaseRaw.db", nil)
	if err != nil {
		return
	} else {
		defer dbR.Close()
	}
	db, err := leveldb.OpenFile("database.db", nil)
	if err != nil {
		return
	} else {
		defer db.Close()
	}

	//Цикл по сырой базе
	iter := dbR.NewIterator(nil, nil)
	for iter.Next() {
		s := bytesToSignal(iter.Value())
		//S2
		ll := s2.LatLngFromDegrees(s.Lat, s.Lng)     //get LatLon for CellID
		cellID := s2.CellIDFromLatLng(ll).Parent(15) // get CellID, set 15 ур
		cell := s2.CellFromCellID(cellID)            // get Cell по

		//Ищем полученную клетку в обычной БД
		cellKey := s2CellIDtoBytes(cellID)
		data, err := db.Get(cellKey, nil)

		if err == leveldb.ErrNotFound {
			//Клетка не найдена
			//Забираем угловые координаты
			vertices := s2.PolygonFromCell(cell).Loop(0).Vertices()
			var coordinates []Coordinate
			for i := 0; i < len(vertices); i++ {
				latlng := s2.LatLngFromPoint(vertices[i])
				coordinate := Coordinate{latlng.Lat.Degrees(), latlng.Lng.Degrees()}
				coordinates = append(coordinates, coordinate)
			}

			//Забираем координаты центра
			center := cell.RectBound().Center()
			newCell := Cell{
				Center:      Coordinate{center.Lat.Degrees(), center.Lng.Degrees()},
				Data:        []Signal{s},
				Coordinates: coordinates,
			}

			data = CelltoBytes(newCell)

		} else if err != nil {
			//Неизвестная ошибка - пропускаем итерацию
			continue
		} else {
			//Клетка существует - дополняем
			oldCell := bytesToCell(data)
			oldCell.Data = append(oldCell.Data, s)
			data = CelltoBytes(oldCell)
		}

		//Записываем модифицированную или новую клетку в БД
		err = db.Put(cellKey, data, nil)
		if err != nil {
			return
		} else {
			defer dbR.Close()
		}

		//Удаляем обаботанный сигнал из "сырой" БД
		err = dbR.Delete(iter.Key(), nil)
		if err != nil {
			return
		} else {
			defer dbR.Close()
		}
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

//Хотелось бы переписать эти две функии с необявленным типом, как это можно сделать в C++, но такого не нашёл
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
