let map
let infoWindow
const APIURL = "http://127.0.0.1:8080/data"
let tiles = []
let tilesRaw = []
let labels = {
	avg: true,
	uniq: true
}
const button = document.getElementById("refresh")

const refreshMap = async () => {
	button.disabled = true

	tiles.forEach(el => {
		el.setMap(null)
	});
	tiles = []
	
	let fetchErr = false
	const bounds = map.getBounds()
	let data = {
		"Area":[
			{
				"lat": bounds.lc.g,
				"lon": bounds.Eb.g
			},
			{
				"lat": bounds.lc.i,
				"lon": bounds.Eb.i
			},
		]
	}
	await fetch(APIURL,{
		method: "POST",
		body: JSON.stringify(data)
	})
		.then(res => res.status == 200 ? res.json(): [])
		.then(res => tilesRaw = res != [] ? res: [])
		.catch(res => {console.log(res); fetchErr = true })
	
	if(fetchErr){
		alert("Oшибка запроса")
		return
	}

	for(let i=0; i < tilesRaw.length; i++){
		const coords = coordinatesFromAPI(tilesRaw[i].S2_coordinates)
		tiles[i] = new google.maps.Polygon({
			uniq_users: tilesRaw[i].Uniq_users,
			signal_avg: tilesRaw[i].Signal_avg,
			paths: coords,
			strokeColor: "#2380F1",
			strokeOpacity: 0.8,
			strokeWeight: 3,
			fillColor: "#2784F5",
			fillOpacity: 0.8 * tilesRaw[i].Signal_avg / 100,
		})
		tiles[i].addListener("click", showArrays)
		tiles[i].setMap(map)
	}
}

const labelHandler = (id) => {
	labels[id] = document.getElementById(id).checked
}
document.getElementById("avg").addEventListener("click", ()=>labelHandler("avg"))
document.getElementById("uniq").addEventListener("click", ()=>labelHandler("uniq"))

function initMap() {
  map = new google.maps.Map(document.getElementById("map"), {
    zoom: 12,
    center: { lat: 59.93428, lng: 30.3351 }
  })
  infoWindow = new google.maps.InfoWindow()
  map.addListener("dragstart", buttonAttention)
  map.addListener("zoom_changed", buttonAttention)
}

function showArrays(event) {
  const signal_avg = labels.avg ? `Средний сигнал: ${this.signal_avg.toFixed(2)}% <br>` : '' 
  const uniq_users = labels.uniq ? `Уникальные пользователи: ${this.uniq_users}` : '' 
  const contentString = signal_avg + uniq_users

  infoWindow.setContent(contentString)
  infoWindow.setPosition(event.latLng)
  infoWindow.open(map)
}

const buttonAttention = () => {
	button.disabled = false	
	//if (!button.classList.contains('attention')) button.classList.add("attention")
}

const coordinatesFromAPI = (value)=>{
	result = []
	value.forEach(el => {
		result.push({
			lat: el.Lat,
			lng: el.Lon
		})
	});
	return result
}  