Block allow lists
=================

rdsys can be configured with two optional files to be able to prevent 
some resources to be distributed in some countries:

* **Blocklist**. Contains the list of resources that should not be 
  distributed on the given country. Requests from a country included
  in the blocklist will not get any resources included in the list.
* **Allowlist**. Countains the list of the only resources that should 
  be distributed in the given country. Requests from a country included
  in the allowlist will only get resources from that list.

Both files have the same line format:

      fingerprint <bridge fingerprint> country-code <country code>

The country code is the two letter code of the country in lower case.

If the same country is listed on both lists the blocklist will be 
ignored and only the allowlist will be used.
