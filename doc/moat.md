Moat distributor
================

The moat distributor is an API that clients use to get bridges and circumvention 
settings. Clients use Domain Fronting to avoid censorship when connecting to the 
API.

There are two different mechanisms to discover bridges in moat:

* Captcha based. Implemented in BridgeDB, provides bridges and uses captchas to 
  protect from attackers. Has being the main mechanism in Tor Browser to 
  discover bridges until Circumvention Settings were added.
* Circumvention Settings. Uses the client location to recommend a pluggable 
  transport to use.

Each mechanism is seen from the rdsys backend as a different distributor (`moat` 
and `settings`) and have a different pool of bridges assigned to it.


Circumvention Settings protections
----------------------------------

The resources provided `/circumvention/settings` and `/circumvention/defaults` 
use a combination of two mechanisms to make it harder for attackers to list all 
the bridges.

Resources are group so each resource will only be distributed in a certain time 
period (`rotation_period_hours`), and will not be distributed again until a 
number of periods has past (`num_periods`). If `rotation_period_hours=24` and 
`num_periods=30` resources will be divided in 30 groups and each group will be 
distributed during one day and so a single resource will not be distributed 
again 30 days has passed.

The IP address of the requester will be used so over the same rotation period 
every IP coming from the same subnet will get the same resources on each 
request.
