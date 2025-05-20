This tool may prevent you from getting a single sided sunburn during a planned trip, or explaines why it happened.

# gpx-sunheadinger

Analyze the input of a GPX track for sun exposure on a trip, e.g. when driving an open roof car or going by bicycle.
As input, it requires a gpx track with relatively close trackpoints with timestamp information.
A command line tool, written in golang. Go runtime must be installed.

The tool anylyzes the GPX track, computes a heading taken and combines this with the angle to the sun, at any given time.
From there, the sum of seconds of sunshine, for each angle of exposure, relative to one's own direction, is calulated, as it happened or is to be expected.
At the end, it provides you with the seconds of sun for each angle of exposure to your body, all summed up for a trip.

Additional features:
- deep sun shows you when the sun is closer to horizon. Forward angles indicate the duration of blinding your view.
- distribution of the circles for quartiles provide an indication of even distribution of sunlight throughout the trip (closer by quartiles: more even distribution) 

Data aquisition proofs to be still challenging, in particular for planned trips.
To analyse a track for the future, use a GPX planning tool like GPSRouteConverter, 
to create a detailed track for the planned route. 
If the tool of your choice does not provide timestamps, add a rough estimation for those to the track e.g. by using gpx-timetagger.

Typical workflow example:
- plan your trip using your planner of choice, e.g. kurviger.com
- export your trip into a GPX as a track (not waypoints or route)

or
- Take your planner GPX with waypoints or a route and convert it to a track using GPSRouteConverter

or
- take your GPX recoding of a track

- use a GPX editor tool like Adze to split track fo rpauses and/or adjust timestamps for the trip as needed

then
- run "go run . <yourtrip.gpx> 10s
- use the generated .gpx and .csv files for your analysis, visualization and planning.

E.g. you use <yourtrip>_0_0.sunimpact for visualization in a radar/spider chart like below, to showcase the seconds of sun exposure per angle towards your car/bike/body.

![sun headinger spider chart illustration](gpx-sunheadinger.png) 
