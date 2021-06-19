const latDiff = 0.10 // to 60.00
const latMin = 59.90
const lngDiff = 0.20 // to 30.40
const lngMin = 30.20
const collectorAPI = "http://127.0.0.1:8080/collector"
let interval
let summ = 0
let errors = 0
let errorsInARow = 0

const fillDB = () => {
    interval = setInterval(()=>{
        if(errorsInARow == 5) {
            stopFilling()
            return}
        let data = {
            lat: parseFloat((Math.random() * latDiff + latMin).toFixed(6)),
            lng: parseFloat((Math.random() * lngDiff + lngMin).toFixed(6)),
            user_id: `9${parseInt(Math.random()*10)}ea80cf-268a-4${parseInt(Math.random()*10)}4f-9ebb-5cc4${parseInt(Math.random()*10)}b55365b`,
            signal: parseFloat((Math.random() * 100).toFixed(2))
        }
        fetch(collectorAPI,{
            method: "POST",
            body: JSON.stringify(data)
        })
        .then(res=>{
            console.log(++summ)
            errorsInARow = 0})
        .catch(err=>{
            console.log(`Ошибка: ${++errors}`)
            errorsInARow++})
    }, 700)
}

const stopFilling = () => {
    summ, errors = 0, 0
    clearInterval(interval)
}

document.getElementById("fillDB").addEventListener("click", fillDB)
document.getElementById("stopFilling").addEventListener("click", stopFilling)