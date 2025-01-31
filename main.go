package main

import (
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/montanaflynn/stats"
	"github.com/sixdouglas/suncalc"
	"github.com/tkrajina/gpxgo/gpx"
)

func usage() {
	fmt.Println("Add heading into a GPX file based on location between track points.")
	fmt.Println("Usage: go run . <example.gpx> [pause detection as duratio e.g. 10s (default)]")
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
	sunPosition := suncalc.GetPosition(dateTime, latitude, longitude)
	return radiansToDegrees(sunPosition.Azimuth), radiansToDegrees(sunPosition.Altitude)
}

// Sun state management
type SunState int

const (
	SunDowned = iota
	SunLow
	SunBlinding
	SunUp
	Unknown
)

var currentSunState SunState

func (sunState SunState) EnumIndex() int {
	return int(sunState)
}

func (sunState SunState) ToString() string {
	switch sunState {
	case SunDowned:
		return "Sun Down"
	case SunLow:
		return "Sun Low"
	case SunBlinding:
		return "Sun Blinding"
	case SunUp:
		return "Sun Up"
	}
	return "Unknown"
}

func (sunState SunState) hasChanged(newSunState SunState) bool {
	if sunState.EnumIndex() == newSunState.EnumIndex() {
		return false
	}
	currentSunState = newSunState
	return true
}

func (sunState SunState) getColorExtension() *gpx.ExtensionNode {
	colorExtension := &gpx.ExtensionNode{
		XMLName: xml.Name{Space: "http://www.garmin.com/xmlschemas/GpxExtensions/v3", Local: "DisplayColor"},
		Data:    "Red",
	}
	extensionNode := &gpx.ExtensionNode{
		XMLName: xml.Name{Space: "http://www.garmin.com/xmlschemas/GpxExtensions/v3", Local: "TrackExtension"},
		Data:    "\n                \n            ",
	}
	extensionNode.Nodes = append(extensionNode.Nodes, *colorExtension)
	return extensionNode
}

func nextTrack(trackIndex int, gpxFile *gpx.GPX, previousPoint *gpx.GPXPoint) *gpx.GPXTrack {
	gpxTrack := &gpx.GPXTrack{
		Name: gpxFile.Tracks[trackIndex].Name + " " + strconv.Itoa(trackIndex) + " " + currentSunState.ToString(),
	}

	gpxTrack.Extensions.Nodes = append(gpxTrack.Extensions.Nodes, *currentSunState.getColorExtension())
	gpxTrack.Number.SetValue(trackIndex)
	if previousPoint != nil {
		gpxTrack.AppendSegment(nextSegment(previousPoint))
	}
	return gpxTrack
}

func nextSegment(previousPoint *gpx.GPXPoint) *gpx.GPXTrackSegment {
	gpxSegment := new(gpx.GPXTrackSegment)
	gpxSegment.AppendPoint(previousPoint)
	return gpxSegment
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}

	pauseDetectDuration, _ := time.ParseDuration("10s")
	if len(os.Args) > 2 {
		pauseDetectDuration, _ = time.ParseDuration(os.Args[2])
	}
	fmt.Println("running with " + pauseDetectDuration.String() + " pause detection")

	// GPX input file
	filename := os.Args[1]
	payload, err := os.ReadFile(filename)
	check(err)

	// parse input from GPX format
	gpxFile, err := gpx.ParseBytes(payload)
	check(err)

	// GPX output file
	gpxOutput := &gpx.GPX{
		Name:             gpxFile.Name,
		Time:             gpxFile.Time,
		Creator:          "EverydayRoadster gpx-sunheadinger",
		AuthorName:       gpxFile.AuthorName,
		AuthorEmail:      gpxFile.AuthorEmail,
		Copyright:        gpxFile.Copyright,
		CopyrightLicense: gpxFile.CopyrightLicense,
	}
	gpxOutput.RegisterNamespace("gpxx", "http://www.garmin.com/xmlschemas/GpxExtensions/v3")
	currentSunState.hasChanged(Unknown)

	// for each track, segments inside track, all points inside each of the segments
	for trackIndex := range gpxFile.Tracks {
		gpxOutput.AppendTrack(nextTrack(trackIndex, gpxFile, nil))

		for segIndex := range gpxFile.Tracks[trackIndex].Segments {
			// initialize data buckets
			// counts of instances, useful on even timespans only, offers direct normalization for easier comparism
			sunImpactDistribution := make([]float64, 360)
			// time of sun exposure per degree of impact to a car
			sunImpactDistributionTime := make([]float64, 360)
			// time of deep standing sun exposure per degree of impact to a car
			deepSunImpactDistributionTime := make([]float64, 360)
			// time of blinding sun exposure per degree of impact to a car
			blindingSunImpactDistributionTime := make([]float64, 360)

			// align filename with name from input file
			filename = filename[0 : len(filename)-len(filepath.Ext(filename))]

			// create csv files for each GPX segment
			csvHeadings, err := os.Create(filename + "_" + strconv.Itoa(trackIndex) + "_" + strconv.Itoa(segIndex) + ".csv")
			check(err)
			csvHeadingsWriter := csv.NewWriter(csvHeadings)
			csvHeadingsWriter.Write([]string{"timestamp", "timegap", "lat", "lon", "carHeading", "sunAzimuth", "sunElevation", "sunImpactAngle"})

			for pointIndex := range gpxFile.Tracks[trackIndex].Segments[segIndex].Points {
				if pointIndex > 0 {
					// check time gap between two subsequent track points
					timegap := gpxFile.Tracks[trackIndex].Segments[segIndex].Points[pointIndex].Timestamp.Sub(gpxFile.Tracks[trackIndex].Segments[segIndex].Points[pointIndex-1].Timestamp)
					// on gap being larger than threshold, ignore this value (pause detection)
					if timegap > pauseDetectDuration {
						continue
					}
					// inputs for calculating the angle between two subsequent track points (car direction)
					phi1 := degreesToRadians(gpxFile.Tracks[trackIndex].Segments[segIndex].Points[pointIndex-1].Latitude)
					lambda1 := degreesToRadians(gpxFile.Tracks[trackIndex].Segments[segIndex].Points[pointIndex-1].Longitude)
					phi2 := degreesToRadians(gpxFile.Tracks[trackIndex].Segments[segIndex].Points[pointIndex].Latitude)
					lambda2 := degreesToRadians(gpxFile.Tracks[trackIndex].Segments[segIndex].Points[pointIndex].Longitude)

					deltaLambda := lambda2 - lambda1
					// skip two dots on same location: no movement, not computable
					// TODO: check for rare case of move along Latitude only, what is done then? Some GPX loggers are poor on resolution....
					if deltaLambda == 0 {
						continue
					}

					leftSide := math.Sin(deltaLambda) * math.Cos(phi2)
					rightSide := (math.Cos(phi1) * math.Sin(phi2)) - (math.Sin(phi1) * math.Cos(phi2) * math.Cos(deltaLambda))
					theta := math.Atan2(leftSide, rightSide)
					// car direction
					carHeading := theta * 180 / math.Pi

					// normalize to 360°
					carHeading = math.Mod(carHeading, 360)
					if carHeading < 0 {
						carHeading = 360 + carHeading
					}

					sunAzimuth, sunElevation := calculateSunPosition(gpxFile.Tracks[trackIndex].Segments[segIndex].Points[pointIndex].Latitude, gpxFile.Tracks[trackIndex].Segments[segIndex].Points[pointIndex].Longitude, gpxFile.Tracks[trackIndex].Segments[segIndex].Points[pointIndex].Timestamp)
					// a sun that is set, it makes it to the graph but not into the statistics
					if sunElevation < 0 {
						if currentSunState.hasChanged(SunDowned) {
							gpxOutput.AppendTrack(nextTrack(trackIndex, gpxFile, &gpxFile.Tracks[trackIndex].Segments[segIndex].Points[pointIndex-1]))
						}
						gpxOutput.AppendPoint(&gpxFile.Tracks[trackIndex].Segments[segIndex].Points[pointIndex])
						continue
					}
					// orientation fix for hemisphere
					if gpxFile.Tracks[trackIndex].Segments[segIndex].Points[pointIndex].Latitude > 0 {
						sunAzimuth += 180
					}
					// normalize to 360°
					sunAzimuth = math.Mod(sunAzimuth, 360)
					if sunAzimuth < 0 {
						sunAzimuth = 360 + sunAzimuth
					}
					// calc sun impact relative to direction of car
					sunImpactAngle := math.Mod(sunAzimuth-carHeading, 360)
					// normalize to 360°
					if sunImpactAngle < 0 {
						sunImpactAngle = 360 + sunImpactAngle
					}

					// collect a value into a stack per degree of sun impact to car direction
					sunImpactDistribution[int(sunImpactAngle)]++

					sunImpactDurationSeconds := gpxFile.Tracks[trackIndex].Segments[segIndex].Points[pointIndex].Timestamp.Sub(gpxFile.Tracks[trackIndex].Segments[segIndex].Points[pointIndex-1].Timestamp).Seconds()
					sunImpactDistributionTime[int(sunImpactAngle)] += sunImpactDurationSeconds

					// deep sun is sun impact below 15° elevation
					if (sunElevation > 0) && (sunElevation < 15) {
						deepSunImpactDistributionTime[int(sunImpactAngle)] += sunImpactDurationSeconds
						// blinding sun is deep sun and +-30° heading north
						if (sunImpactAngle < 30) || (sunImpactAngle > 330) {
							blindingSunImpactDistributionTime[int(sunImpactAngle)] += sunImpactDurationSeconds
							if currentSunState.hasChanged(SunBlinding) {
								gpxOutput.AppendTrack(nextTrack(trackIndex, gpxFile, &gpxFile.Tracks[trackIndex].Segments[segIndex].Points[pointIndex-1]))
							}
						} else {
							if currentSunState.hasChanged(SunLow) {
								gpxOutput.AppendTrack(nextTrack(trackIndex, gpxFile, &gpxFile.Tracks[trackIndex].Segments[segIndex].Points[pointIndex-1]))
							}
						}
					} else {
						if currentSunState.hasChanged(SunUp) {
							gpxOutput.AppendTrack(nextTrack(trackIndex, gpxFile, &gpxFile.Tracks[trackIndex].Segments[segIndex].Points[pointIndex-1]))
						}
					}
					gpxOutput.AppendPoint(&gpxFile.Tracks[trackIndex].Segments[segIndex].Points[pointIndex])

					// write raw stuff
					csvHeadingsWriter.Write([]string{
						gpxFile.Tracks[trackIndex].Segments[segIndex].Points[pointIndex].Timestamp.String(),
						timegap.String(),
						strconv.FormatFloat(gpxFile.Tracks[trackIndex].Segments[segIndex].Points[pointIndex].Latitude, 'f', 6, 64),
						strconv.FormatFloat(gpxFile.Tracks[trackIndex].Segments[segIndex].Points[pointIndex].Longitude, 'f', 6, 64),
						strconv.FormatFloat(carHeading, 'f', 6, 64),
						strconv.FormatFloat(sunAzimuth, 'f', 6, 64),
						strconv.FormatFloat(sunElevation, 'f', 6, 64),
						strconv.FormatFloat(sunImpactAngle, 'f', 6, 64)})
				}
			}
			csvHeadingsWriter.Flush()
			csvHeadings.Close()

			// compute quartiles
			quartiles, err := stats.Quartile(sunImpactDistributionTime)
			check(err)
			interQuartileRange, err := stats.InterQuartileRange(sunImpactDistributionTime)
			check(err)
			maxSunImpactTime, err := stats.Max(sunImpactDistributionTime)
			check(err)
			sumSunBlinding, err := stats.Sum(blindingSunImpactDistributionTime)
			check(err)

			fmt.Println("Track: " + strconv.Itoa(trackIndex) + " Segment: " + strconv.Itoa(segIndex) + " Timed InterQuartileRange: " + strconv.FormatFloat(interQuartileRange, 'f', 0, 64) + ", Peak factor: " + strconv.FormatFloat(maxSunImpactTime/interQuartileRange, 'f', 2, 64) + ", blinding for " + strconv.FormatFloat(sumSunBlinding/60, 'f', 2, 64) + " minutes.")

			// write collected data stuff
			csvSunImpact, err := os.Create(filename + "_" + strconv.Itoa(trackIndex) + "_" + strconv.Itoa(segIndex) + ".sunimpact.csv")
			check(err)
			csvSunImpactWriter := csv.NewWriter(csvSunImpact)
			csvSunImpactWriter.Write([]string{"Impact Angle", "count", "normalized count", "timesum sun", "timesum deep sun", "Q1 timed", "Q2 timed", "Q3 timed"})

			// max, to normalize to 100 slices.Max()
			maxSunImpactDistribution := slices.Max(sunImpactDistribution)
			for carAngleIndex := range sunImpactDistributionTime {
				csvSunImpactWriter.Write([]string{
					strconv.Itoa(carAngleIndex),
					strconv.FormatFloat(sunImpactDistribution[carAngleIndex], 'f', 2, 64),
					strconv.FormatFloat(sunImpactDistribution[carAngleIndex]*100/maxSunImpactDistribution, 'f', 2, 64),
					strconv.FormatFloat(sunImpactDistributionTime[carAngleIndex], 'f', 2, 64),
					strconv.FormatFloat(deepSunImpactDistributionTime[carAngleIndex], 'f', 2, 64),
					strconv.FormatFloat(quartiles.Q1, 'f', 2, 64),
					strconv.FormatFloat(quartiles.Q2, 'f', 2, 64),
					strconv.FormatFloat(quartiles.Q3, 'f', 2, 64)})
			}
			csvSunImpactWriter.Flush()
			csvSunImpact.Close()
		}
	}
	// create output GPX file
	xmlBytes, err := gpxOutput.ToXml(gpx.ToXmlParams{Version: "1.1", Indent: true})
	check(err)
	// write GPX XML output
	err = os.WriteFile(filename+".sunstatus.gpx", xmlBytes, 0666)
	check(err)

}
