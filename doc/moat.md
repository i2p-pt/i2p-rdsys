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
and `settings`) and has a different pool of bridges assigned to it.

[[_TOC_]]


Circumvention Settings protections
----------------------------------

The resources provided `/circumvention/settings` and `/circumvention/defaults` 
use a combination of two mechanisms to make it harder for attackers to list all 
the bridges.

Resources are grouped so each resource will only be distributed in a certain 
time period (`rotation_period_hours`), and will not be distributed again until a 
number of periods has passed (`num_periods`). If `rotation_period_hours=24` and 
`num_periods=30`, resources will be divided in 30 groups, and each group will be 
distributed during one day. A single resource will not be distributed again 
until 30 days has passed.

The IP address of the requester will be used so over the same rotation period 
every IP coming from the same subnet will get the same resources on each 
request.

API
---

The moat API is located in https://bridges.torproject.org/moat/ and reachable 
over domain fronting. All endpoints described here are located under the moat 
url, for example */fetch* full url will be 
https://bridges.torproject.org/moat/fetch.

They will always get a *HTTP Status 200* response with a json object, whether or 
not the request was valid or had produced an error.

### Error responses

If an error is produced the response json will contain a list of errors with the 
following form:
```json
{
  "errors": [
    {
      "code": 400,
      "detail": "Not valid request"
    }
  ]
}
```

Where the *code* field contains a numeric code representing the error and 
*detail* a human readable description of the problem.

### Captcha based endpoints

Provides bridges using a captcha as a protection mechanism.

All captcha based requests require of the `Content-Type` HTTP header being set 
to `application/vnd.api+json` or the response will contain an error code 
**415**. Requests have to be an *HTTP POST* with an optional json body. 

#### /fetch

Fetch a captcha challenge to get bridges.

##### request

An optional body can be provided in the request with the list of supported 
transports by the client:
```json
{
  "data": [
    {
      "type": "client-transports",
      "version": "0.1.0",
      "supported": [
	"obfs4"
      ]
    }
  ]
}
```

##### response

```json
{
  "data": [
    {
      "id": "1",
      "type": "moat-challenge",
      "version": "0.1.0",
      "transport": [
        "obfs4",
        "vanilla"
      ],
      "image": "xxxxxx",
      "challenge": "xxxxx"
    }
  ]
}
```

* `image` contains the base64 encoded jpg image of the captcha.
* `challenge` is an unique string associated with the request.
* `transport` list of valid transports in the preferred order.




##### examples

```
$ curl -X POST -H 'Content-Type: application/vnd.api+json' https://bridges.torproject.org/moat/fetch
{
  "data": [
    {
      "id": "1",
      "type": "moat-challenge",
      "version": "0.1.0",
      "transport": [
        "obfs4",
        "vanilla"
      ],
      "image": "xxxx",
      "challenge": "xxxxx"
    }
  ]
}
$ curl -X POST -H 'Content-Type: application/vnd.api+json' -d '{"data": [{"type": "client-transports", "version": "0.1.0", "supported": ["obfs4"]}]}' https://bridges.torproject.org/moat/fetch
{
  "data": [
    {
      "id": "1",
      "type": "moat-challenge",
      "version": "0.1.0",
      "transport": "obfs4",
      "image": "xxxx",
      "challenge": "xxxxx"
    }
  ]
}
```

#### /check

Send the solution of the captcha to get bridges.

##### request 

The request must include a body with the solution of the captcha:
```json
{
  "data": [
    {
      "type": "moat-solution",
      "id": "2",
      "version": "0.1.0",
      "transport": "obfs4",
      "challenge": "xxxx",
      "qrcode": "false",
      "solution": "yyyy"
    }
  ]
}
```

* `solution` contains the solution of the captcha.
* `challenge` is the unique string provided in by the /fetch endpoint.
* `transport` the transport type of the bridges we want to get.
* `qrcode` if the response should include a qrcode of the bridgelines.
* `id` must be set to *2*.

##### response

```json
{
  "data": [
    {
      "id": "3",
      "type": "moat-bridges",
      "version": "0.1.0",
      "bridges": [
        "obfs4 x.x.x.x:x AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA cert=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa iat-mode=0",
        "obfs4 x.x.x.x:x AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA cert=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa iat-mode=0"
      ],
      "qrcode": null
    }
  ]
}
```

* `bridges` the list of bridgelines returned.
* `qrcode` if the qrcode has being requested contains the base64 of the jpg.

##### error

If the solution of the challenge is not valid or the challenge has expired it 
will respond with an error code **419**.

```json
{
  "errors": [
    {
      "id": "4",
      "type": "moat-bridges",
      "version": "0.1.0",
      "code": 419,
      "status": "No You're A Teapot",
      "detail": "The CAPTCHA solution was incorrect."
    }
  ]
}
```


##### examples

```
$ curl $CURL_OPTIONS -H 'Content-Type: application/vnd.api+json' -d '{"data": [{"type": "moat-solution", "id": "2", "version": "0.1.0", "transport": "obfs4", "challenge": "xxx", "qrcode": "true", "solution": "aaaaaaa"}]}" -X POST https://bridges.torproject.org/moat/check
{
  "data": [
    {
      "id": "3",
      "type": "moat-bridges",
      "version": "0.1.0",
      "bridges": [
        "obfs4 x.x.x.x:x AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA cert=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa iat-mode=0",
        "obfs4 x.x.x.x:x AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA cert=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa iat-mode=0"
      ],
      "qrcode": "data:image/jpeg;base64,b'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'"
    }
  ]
}

```

### Circumvention Settings endpoints

The Circumvention Settings endpoints are used to gather information on different 
circumvention mechanisms and what work on each country.

Most requests to the Circumvention Settings endpoints are to use the HTTP POST 
method with an optional json payload. Some requests that don't take arguments 
can be used as GET requests, for example /circumvention/builtin and 
/circumvention/countries.

#### /circumvention/settings

Uses the location of the requester to give a list of circumvention mechanism 
that works on the location and specific information on how to configure them. If 
the country code is not provided in the request body, it will discover the 
requester location from its IP address.

##### request

An optional request body can be provided with the following fields:
* `country` indicates the country where the client is located. Used when the 
  user inputs the country manually instead of using the geolocation service.
* `transports` a list of supported transports by the client. Only supported 
  transports will be returned in the settings response.

The fields are optional, and can be provided individually or as a combination in 
the same payload.

```json
{
  "country": "de",
  "transports": ["obfs4", "snowflake"]
}
```

##### response

The response will be a json with the following structure:

```json
{
  "settings": [
    {
      "bridges": {
        "type": "obfs4",
        "source": "builtin",
        "bridge_strings": [
          "obfs4 209.148.46.65:443 74FAD13168806246602538555B5521A0383A1875 cert=ssH+9rP8dG2NLDN2XuFw63hIO/9MNNinLmxQDpVa+7kTOa9/m+tGWT1SmSYpQ9uTBGa6Hw iat-mode=0",
          "obfs4 38.229.33.83:80 0BAC39417268B96B9F514E7F63FA6FBA1A788955 cert=VwEFpk9F/UN9JED7XpG1XOjm/O8ZCXK80oPecgWnNDZDv5pdkhq1OpbAH0wNqOT6H6BmRQ iat-mode=1"
        ]
      }
    },
    {
      "bridges": {
        "type": "obfs4",
        "source": "bridgedb",
        "bridge_strings": [
          "obfs4 x.x.x.x:x AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA cert=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa iat-mode=0",
          "obfs4 x.x.x.x:x AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA cert=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa iat-mode=0"
        ]
      }
    },
    {
      "bridges": {
        "type": "snowflake",
        "source": "builtin",
        "bridge_strings": [
          "snowflake 192.0.2.3:1 2B280B23E1107BB62ABFC40DDCC8824814F80A72"
        ]
      }
    }
  ],
  "country": "de"
}
```
The `settings` list is sorted by the most useful circumvention mechanism first 
for the location. Each `bridges` entry contains the following fields:
* `type` the transport type.
* `source` the source of the bridges to be used. It can be `builtin` for bridges 
  that are publicly included by the client or `bridgedb` for bridges that are 
  not publicly provided just for this client to use.
* `bridge_strings` a list of bridgelines for the client to use.

The `country` is the country code for which those settings are. If no country 
was provided in the request this will be the country discovered from the IP 
address of the requester.

If Tor should work on the location without any circumvention mechanism the 
response will contain an empty list in the `settings` field:
```json
{
  "settings": [],
  "country": "se"
}
```

##### error

The possible error codes in the answer are:
* **400** the request body is not correct:
```json
{
  "errors": [
    {
      "code": 400,
      "detail": "Not valid request"
    }
  ]
}
```

* **404** the location needs transports but none of the provided ones in the 
  request will work:
```json
{
  "errors": [
    {
      "code": 404,
      "detail": "No provided transport is available for this country"
    }
  ]
}
```

* **406** moat can't determine the country from the IP address:
```json
{
  "errors": [
    {
      "code": 406,
      "detail": "Could not find country code for circumvention settings"
    }
  ]
}
```

##### examples

```
$ curl https://bridges.torproject.org/moat/circumvention/settings
{}
$ curl -d '{"country": "cn"}' https://bridges.torproject.org/moat/circumvention/settings
{
  "settings": [
    {
      "bridges": {
        "type": "snowflake",
        "source": "builtin",
        "bridge_strings": [
          "snowflake 192.0.2.3:1 2B280B23E1107BB62ABFC40DDCC8824814F80A72"
        ]
      }
    }
  ]
}
$ curl -d '{"transports": "obfs4"}' https://bridges.torproject.org/moat/circumvention/settings
{
  "errors": [
    {
      "code": 400,
      "detail": "Not valid request"
    }
  ]
}
$ curl -d '{"country": "cn", "transports": ["obfs4"]}' https://bridges.torproject.org/moat/circumvention/settings
{
  "errors": [
    {
      "code": 404,
      "detail": "No provided transport is available for this country"
    }
  ]
}
```

#### /circumvention/defaults

Provides a list of default settings to be used if the client can't connect to 
Tor without circumvention mechanism but there are no specific settings for the 
location.

The request, response and errors are the same as for `/circumvention/settings`. 
With the following differences:
* The request body only accepts the `transports` field and no `country` field.
* The error code **406** will not be returned by this endpoint.

#### /circumvention/map

Responds with the current knowledge of the circumvention mechanisms that works 
for each location.

##### response

```json
{
  "cn": {
    "settings": [
      {
        "bridges": {
          "type": "snowflake",
          "source": "builtin"
        }
      }
    ]
  },
  "ru": {
    "settings": [
      {
        "bridges": {
          "type": "snowflake",
          "source": "builtin"
        }
      },
      {
        "bridges": {
          "type": "obfs4",
          "source": "bridgedb"
        }
      }
    ]
  }
}
```

The json contains the country code and the settings that applies to it. The 
fields are the same as for `/circumvention/settings` but the map doesn't provide 
`bridge_strings`.

##### examples

```
$ curl https://bridges.torproject.org/moat/circumvention/map
{
  "by": {
    "settings": [
      {
        "bridges": {
          "type": "obfs4",
          "source": "builtin"
        }
      },
      {
        "bridges": {
          "type": "vanilla",
          "source": "bridgedb"
        }
      },
      {
        "bridges": {
          "type": "obfs4",
          "source": "bridgedb"
        }
      },
      {
        "bridges": {
          "type": "snowflake",
          "source": "builtin"
        }
      }
    ]
  },
  "cn": {
    "settings": [
      {
        "bridges": {
          "type": "snowflake",
          "source": "builtin"
        }
      }
    ]
  },
  "ru": {
    "settings": [
      {
        "bridges": {
          "type": "snowflake",
          "source": "builtin"
        }
      },
      {
        "bridges": {
          "type": "obfs4",
          "source": "bridgedb"
        }
      }
    ]
  },
  "tm": {
    "settings": [
      {
        "bridges": {
          "type": "obfs4",
          "source": "bridgedb"
        }
      },
      {
        "bridges": {
          "type": "snowflake",
          "source": "builtin"
        }
      }
    ]
  }
}
```

#### /circumvention/builtin

Provides the full list of [builtin 
bridges](https://gitlab.torproject.org/tpo/anti-censorship/team/-/wikis/Default-Bridges) 
currently in use. Builtin bridges are public bridges often included in the 
client.

##### response

```json
{
  "meek-azure": [
    "meek_lite 192.0.2.2:2 97700DFE9F483596DDA6264C4D7DF7641E1E39CE url=https://meek.azureedge.net/ front=ajax.aspnetcdn.com"
  ],
  "obfs4": [
    "obfs4 51.222.13.177:80 5EDAC3B810E12B01F6FD8050D2FD3E277B289A08 cert=2uplIpLQ0q9+0qMFrK5pkaYRDOe460LL9WHBvatgkuRr/SL31wBOEupaMMJ6koRE6Ld0ew iat-mode=0",
    "obfs4 209.148.46.65:443 74FAD13168806246602538555B5521A0383A1875 cert=ssH+9rP8dG2NLDN2XuFw63hIO/9MNNinLmxQDpVa+7kTOa9/m+tGWT1SmSYpQ9uTBGa6Hw iat-mode=0"
  ]
}
```

The json object contains an entry for each transport type with a list of 
bridgelines for that transport.


##### examples

```
$ curl  https://bridges.torproject.org/moat/circumvention/builtin
{
  "meek-azure": [
    "meek_lite 192.0.2.2:2 97700DFE9F483596DDA6264C4D7DF7641E1E39CE url=https://meek.azureedge.net/ front=ajax.aspnetcdn.com"
  ],
  "obfs4": [
    "obfs4 51.222.13.177:80 5EDAC3B810E12B01F6FD8050D2FD3E277B289A08 cert=2uplIpLQ0q9+0qMFrK5pkaYRDOe460LL9WHBvatgkuRr/SL31wBOEupaMMJ6koRE6Ld0ew iat-mode=0",
    "obfs4 209.148.46.65:443 74FAD13168806246602538555B5521A0383A1875 cert=ssH+9rP8dG2NLDN2XuFw63hIO/9MNNinLmxQDpVa+7kTOa9/m+tGWT1SmSYpQ9uTBGa6Hw iat-mode=0",
    "obfs4 192.95.36.142:443 CDF2E852BF539B82BD10E27E9115A31734E378C2 cert=qUVQ0srL1JI/vO6V6m/24anYXiJD3QP2HgzUKQtQ7GRqqUvs7P+tG43RtAqdhLOALP7DJQ iat-mode=1",
    "obfs4 37.218.245.14:38224 D9A82D2F9C2F65A18407B1D2B764F130847F8B5D cert=bjRaMrr1BRiAW8IE9U5z27fQaYgOhX1UCmOpg2pFpoMvo6ZgQMzLsaTzzQNTlm7hNcb+Sg iat-mode=0",
    "obfs4 38.229.33.83:80 0BAC39417268B96B9F514E7F63FA6FBA1A788955 cert=VwEFpk9F/UN9JED7XpG1XOjm/O8ZCXK80oPecgWnNDZDv5pdkhq1OpbAH0wNqOT6H6BmRQ iat-mode=1",
    "obfs4 193.11.166.194:27025 1AE2C08904527FEA90C4C4F8C1083EA59FBC6FAF cert=ItvYZzW5tn6v3G4UnQa6Qz04Npro6e81AP70YujmK/KXwDFPTs3aHXcHp4n8Vt6w/bv8cA iat-mode=0",
    "obfs4 38.229.1.78:80 C8CBDB2464FC9804A69531437BCF2BE31FDD2EE4 cert=Hmyfd2ev46gGY7NoVxA9ngrPF2zCZtzskRTzoWXbxNkzeVnGFPWmrTtILRyqCTjHR+s9dg iat-mode=1",
    "obfs4 193.11.166.194:27020 86AC7B8D430DAC4117E9F42C9EAED18133863AAF cert=0LDeJH4JzMDtkJJrFphJCiPqKx7loozKN7VNfuukMGfHO0Z8OGdzHVkhVAOfo1mUdv9cMg iat-mode=0",
    "obfs4 193.11.166.194:27015 2D82C2E354D531A68469ADF7F878FA6060C6BACA cert=4TLQPJrTSaDffMK7Nbao6LC7G9OW/NHkUwIdjLSS3KYf0Nv4/nQiiI8dY2TcsQx01NniOg iat-mode=0",
    "obfs4 45.145.95.6:27015 C5B7CD6946FF10C5B3E89691A7D3F2C122D2117C cert=TD7PbUO0/0k6xYHMPW3vJxICfkMZNdkRrb63Zhl5j9dW3iRGiCx0A7mPhe5T2EDzQ35+Zw iat-mode=0",
    "obfs4 [2a0c:4d80:42:702::1]:27015 C5B7CD6946FF10C5B3E89691A7D3F2C122D2117C cert=TD7PbUO0/0k6xYHMPW3vJxICfkMZNdkRrb63Zhl5j9dW3iRGiCx0A7mPhe5T2EDzQ35+Zw iat-mode=0",
    "obfs4 146.57.248.225:22 10A6CD36A537FCE513A322361547444B393989F0 cert=K1gDtDAIcUfeLqbstggjIw2rtgIKqdIhUlHp82XRqNSq/mtAjp1BIC9vHKJ2FAEpGssTPw iat-mode=0",
    "obfs4 85.31.186.98:443 011F2599C0E9B27EE74B353155E244813763C3E5 cert=ayq0XzCwhpdysn5o0EyDUbmSOx3X/oTEbzDMvczHOdBJKlvIdHHLJGkZARtT4dcBFArPPg iat-mode=0",
    "obfs4 144.217.20.138:80 FB70B257C162BF1038CA669D568D76F5B7F0BABB cert=vYIV5MgrghGQvZPIi1tJwnzorMgqgmlKaB77Y3Z9Q/v94wZBOAXkW+fdx4aSxLVnKO+xNw iat-mode=0",
    "obfs4 85.31.186.26:443 91A6354697E6B02A386312F68D82CF86824D3606 cert=PBwr+S8JTVZo6MPdHnkTwXJPILWADLqfMGoVvhZClMq/Urndyd42BwX9YFJHZnBB3H0XCw iat-mode=0"
  ],
  "snowflake": [
    "snowflake 192.0.2.3:1 2B280B23E1107BB62ABFC40DDCC8824814F80A72"
  ]
}
```

#### /circumvention/countries

Provides the list of country codes for which we know circumvention is needed to 
connect to Tor.

##### examples

```
$ curl  https://bridges.torproject.org/moat/circumvention/countries
[
  "by",
  "cn",
  "tm",
  "ru"
]
```
