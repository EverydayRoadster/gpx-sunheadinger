# gpx-sunheadinger
Command line tool to analyze a gpx track for sun exposure while driving (a cabriolet).
As input, it requires a gpx track with lat/lon and timestamp information.

It will anylyze the track for a heading taken and combines it with angle to the sun at any given time,
calculating the distribution from where exposure to the sun dis happen or is expected.

To analyse a track for the future, use a GPX planning tool like GPSRouteConverter, 
to create a detailed track for the planned route, 
then add timestamps to the track e.g. by using gpx-timetagger.

This tool may prevent you from getting a sigle sided sunburn.
