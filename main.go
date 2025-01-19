package main

import (
	"os"
	"fmt"
	"math"
	"time"
	"slices"
	"strconv"
	"github.com/montanaflynn/stats"
	"github.com/tkrajina/gpxgo/gpx"
	"encoding/csv"
)

func usage() {
	fmt.Println("Add heading into a GPX file based on location between track points.")
	fmt.Println("Usage: go run . example.gpx ")
	os.Exit(0)
}

func check(e error) {
    if e != nil {
        panic(e)
    }
}


// Helper function to convert degrees to radians
func degreesToRadians(degrees float64) float64 {
	return degrees * math.Pi / 180
}

// Helper function to convert radians to degrees
func radiansToDegrees(radians float64) float64 {
	return radians * 180 / math.Pi
}


// Calculate the Sun's position (azimuth and elevation) using Meeus formula
func calculateSunPosition(latitude, longitude float64, dateTime time.Time) (azimuth, elevation float64) {
	// Convert time to UTC and extract year, month, day, hour, minute, second
	utc := dateTime.UTC()
	year, month, day := utc.Date()
	hour, minute, second := utc.Clock()

	// Julian Date
	julianDay := float64((1461*(year+4800+(int(month)-14)/12))/4+
		(367*(int(month)-2-12*((int(month)-14)/12)))/12-
		(3*((year+4900+(int(month)-14)/12)/100))/4+
		day-32075) +
		(float64(hour)-12.0)/24.0 +
		float64(minute)/1440.0 +
		float64(second)/86400.0

	// Julian Century
	julianCentury := (julianDay - 2451545.0) / 36525.0

	// Solar coordinates
	geomMeanLongSun := math.Mod(280.46646+julianCentury*(36000.76983+julianCentury*0.0003032), 360.0)
	geomMeanAnomSun := 357.52911 + julianCentury*(35999.05029-0.0001537*julianCentury)
	eccentEarthOrbit := 0.016708634 - julianCentury*(0.000042037+0.0000001267*julianCentury)

	sunEqOfCtr := math.Sin(degreesToRadians(geomMeanAnomSun))*(1.914602-julianCentury*(0.004817+0.000014*julianCentury)) +
		math.Sin(degreesToRadians(2*geomMeanAnomSun))*(0.019993-0.000101*julianCentury) +
		math.Sin(degreesToRadians(3*geomMeanAnomSun))*0.000289

	sunTrueLong := geomMeanLongSun + sunEqOfCtr
	sunAppLong := sunTrueLong - 0.00569 - 0.00478*math.Sin(degreesToRadians(125.04-1934.136*julianCentury))

	meanObliqEcliptic := 23.0 + (26.0+((21.448-julianCentury*(46.815+julianCentury*(0.00059-julianCentury*0.001813))))/60.0)/60.0
	obliqCorr := meanObliqEcliptic + 0.00256*math.Cos(degreesToRadians(125.04-1934.136*julianCentury))

	// Declination of the Sun
	sunDeclination := radiansToDegrees(math.Asin(math.Sin(degreesToRadians(obliqCorr)) * math.Sin(degreesToRadians(sunAppLong))))

	// Equation of time
	varY := math.Tan(degreesToRadians(obliqCorr/2.0)) * math.Tan(degreesToRadians(obliqCorr/2.0))
	eqOfTime := 4.0 * radiansToDegrees(varY*math.Sin(2.0*degreesToRadians(geomMeanLongSun)) -
		2.0*eccentEarthOrbit*math.Sin(degreesToRadians(geomMeanAnomSun)) +
		4.0*eccentEarthOrbit*varY*math.Sin(degreesToRadians(geomMeanAnomSun))*math.Cos(2.0*degreesToRadians(geomMeanLongSun)) -
		0.5*varY*varY*math.Sin(4.0*degreesToRadians(geomMeanLongSun)) -
		1.25*eccentEarthOrbit*eccentEarthOrbit*math.Sin(2.0*degreesToRadians(geomMeanAnomSun)))

	// Solar Noon
	timeOffset := eqOfTime - 4.0*longitude + 60.0*float64(utc.Hour())/60.0
	trueSolarTime := float64((hour*60 + minute)) + timeOffset

	// Hour angle
	hourAngle := trueSolarTime/4.0 - 180.0
	if hourAngle < -180 {
		hourAngle += 360.0
	}

	// Solar zenith angle
	solarZenithAngle := radiansToDegrees(math.Acos(math.Sin(degreesToRadians(latitude))*math.Sin(degreesToRadians(sunDeclination)) +
		math.Cos(degreesToRadians(latitude))*math.Cos(degreesToRadians(sunDeclination))*math.Cos(degreesToRadians(hourAngle))))

	// Solar elevation angle
	elevation = 90.0 - solarZenithAngle

	// Solar azimuth angle
	if hourAngle > 0 {
		azimuth = math.Mod(radiansToDegrees(math.Acos(((math.Sin(degreesToRadians(latitude))*math.Cos(degreesToRadians(solarZenithAngle))) -
			math.Sin(degreesToRadians(sunDeclination))) / (math.Cos(degreesToRadians(latitude)) * math.Sin(degreesToRadians(solarZenithAngle))))), 360.0)
	} else {
		azimuth = math.Mod(180.0-radiansToDegrees(math.Acos(((math.Sin(degreesToRadians(latitude))*math.Cos(degreesToRadians(solarZenithAngle))) -
			math.Sin(degreesToRadians(sunDeclination))) / (math.Cos(degreesToRadians(latitude)) * math.Sin(degreesToRadians(solarZenithAngle)))))+180.0, 360.0)
	}

	return azimuth, elevation
}


func main(){
	if len(os.Args) < 1 {
		usage()
	}

	// GPX input file
	filename := os.Args[1]
	payload,err := os.ReadFile(filename)
	check(err)

	// parse input from GPX format
	gpxFile,err := gpx.ParseBytes(payload)
	check(err)

	// for each track, segments inside track, all points inside each of the segments
	for trackIndex, _ := range gpxFile.Tracks {
		for segIndex, _ := range gpxFile.Tracks[trackIndex].Segments {

			sunImpactDistributionTime := make([]float64, 360)
			sunImpactDistribution := make([]float64, 360)
			deepSunImpactDistribution := make([]float64, 360)

			csvHeadings, err := os.Create(filename + "_" + strconv.Itoa(trackIndex) + "_" + strconv.Itoa(segIndex) + ".csv")
			check(err)
			csvHeadingsWriter := csv.NewWriter(csvHeadings)
			csvHeadingsWriter.Write([]string{"timestamp", "carHeading", "sunAzimuth", "sunElevation", "sunImpactAngle"})

			for pointIndex, _ := range gpxFile.Tracks[trackIndex].Segments[segIndex].Points {
				if pointIndex > 0 {

					phi1 := degreesToRadians(gpxFile.Tracks[trackIndex].Segments[segIndex].Points[pointIndex-1].Latitude)
					lambda1 := degreesToRadians(gpxFile.Tracks[trackIndex].Segments[segIndex].Points[pointIndex-1].Longitude)
					phi2 := degreesToRadians(gpxFile.Tracks[trackIndex].Segments[segIndex].Points[pointIndex].Latitude)
					lambda2 := degreesToRadians(gpxFile.Tracks[trackIndex].Segments[segIndex].Points[pointIndex].Longitude)

					deltaLambda := lambda2 - lambda1

					leftSide := math.Sin(deltaLambda) * math.Cos(phi2)
					rightSide := (math.Cos(phi1) * math.Sin(phi2)) - (math.Sin(phi1) * math.Cos(phi2) * math.Cos(deltaLambda))
					theta := math.Atan2(leftSide, rightSide)
					carHeading := theta * 180 / math.Pi

					if ( carHeading < 0 ){
						carHeading = 360 + carHeading
					}

					sunAzimuth, sunElevation := calculateSunPosition(phi2, lambda2, gpxFile.Tracks[trackIndex].Segments[segIndex].Points[pointIndex].Timestamp)
					// poor orientation fix
					sunAzimuth = sunAzimuth - (2*(sunAzimuth-180))

					sunImpactAngle := carHeading - sunAzimuth
					if ( sunImpactAngle < 0 ){
						sunImpactAngle = 360 + sunImpactAngle
					}

					sunImpactDistribution[int(sunImpactAngle)]++
					sunImpactDistributionTime[int(sunImpactAngle)] = 
						sunImpactDistributionTime[int(sunImpactAngle)] + gpxFile.Tracks[trackIndex].Segments[segIndex].Points[pointIndex].Timestamp.Sub(gpxFile.Tracks[trackIndex].Segments[segIndex].Points[pointIndex-1].Timestamp).Seconds()
					if (sunElevation >0 ) && (sunElevation <10){
						deepSunImpactDistribution[int(sunImpactAngle)]++
					}

					csvHeadingsWriter.Write([]string{
						gpxFile.Tracks[trackIndex].Segments[segIndex].Points[pointIndex].Timestamp.String(), 
						strconv.FormatFloat(carHeading, 'f', 6, 64), 
						strconv.FormatFloat(sunAzimuth, 'f', 6, 64), 
						strconv.FormatFloat(sunElevation, 'f', 6, 64), 
						strconv.FormatFloat(sunImpactAngle, 'f', 6, 64) } )
				}
			}
			csvHeadingsWriter.Flush()
			csvHeadings.Close()

			quartiles, err := stats.Quartile(sunImpactDistributionTime)
			check(err)
			interQuartileRange, err := stats.InterQuartileRange(sunImpactDistributionTime)
			check(err)
			fmt.Println("Timed Quartiles Q1: " + strconv.FormatFloat(quartiles.Q1, 'f', 0, 64) + ", Q2: " + strconv.FormatFloat(quartiles.Q2, 'f', 0, 64) + ", Q3: " + strconv.FormatFloat(quartiles.Q3, 'f', 0, 64))
			fmt.Println("TImed InterQuartileRange: " + strconv.FormatFloat(interQuartileRange, 'f', 0, 64))

			csvSunImpact, err := os.Create(filename + "_" + strconv.Itoa(trackIndex) + "_" + strconv.Itoa(segIndex) + ".sunimpact.csv")
			check(err)
			csvSunImpactWriter := csv.NewWriter(csvSunImpact)
			csvSunImpactWriter.Write([]string{"Impact Angle", "count", "normalized count", "timesum", "deep sun", "Q1 timed", "Q2 timed", "Q3 timed"})

			// max, to normalize to 100 slices.Max()
			maxSunImpactDistribution := slices.Max(sunImpactDistribution)

			for carAngleIndex, _ := range sunImpactDistributionTime {
				csvSunImpactWriter.Write([]string{
					strconv.Itoa(carAngleIndex), 
					strconv.FormatFloat(sunImpactDistribution[carAngleIndex], 'f', 2, 64), 
					strconv.FormatFloat(sunImpactDistribution[carAngleIndex]*100/maxSunImpactDistribution, 'f', 2, 64), 
					strconv.FormatFloat(sunImpactDistributionTime[carAngleIndex], 'f', 0, 64),
					strconv.FormatFloat(deepSunImpactDistribution[carAngleIndex], 'f', 2, 64),
					strconv.FormatFloat(quartiles.Q1, 'f', 2, 64),
					strconv.FormatFloat(quartiles.Q2, 'f', 2, 64),
					strconv.FormatFloat(quartiles.Q3, 'f', 2, 64) })
			}
			csvSunImpactWriter.Flush()
			csvSunImpact.Close()

		}	
	}

}