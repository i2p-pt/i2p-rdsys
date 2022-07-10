Implementing new Updaters
=========================

This document explains the process of building a new rdsys updater which extends
gettor.

1. [Interface Implementation](https://gitlab.torproject.org/tpo/anti-censorship/rdsys/-/tree/master/pkg/presentation/updaters/gettor) Updaters implement the private [`provider` interface](https://gitlab.torproject.org/tpo/anti-censorship/rdsys/-/tree/master/pkg/presentation/updaters/gettor/gettor.go#L40-L43), which requires makes them easies to implement in Go as part of rdsys itself[*](https://github.com/i2p-pt/i2p-rdsys/blob/main/doc/README.md).

2. Simple configuration and command line code.

Implementation Notes:
---------------------

In order to implement the provider interface, you will need to create a new struct
in `pkg/presentation/updaters/gettor` which implements the following 2 functions:

```go
type provider interface {
	needsUpdate(platform string, version resources.Version) bool
	newRelease(platform string, version resources.Version) uploadFileFunc
}
```

`needsUpdate` is used to determine if your provider needs to be updated corresponding
to a new release of the Tor Browser for the `platform string`. The `version` string
must be compared to the most recent release supported by the provider in order to
implement the required functionality, and return true if the software needs an update.

`newRelease` is used to set up a new release when one is necessary. It is what sets up
the resource which the user will ultimately fetch and use. For example, if the updater
utilizes a file-hosting service, then this updates the content of the file-hosting
service.

```md
* Because of the experimental nature of the i2p-rdsys adaptation, it was necessary to fork
rdsys to add this functionality for testing purposes. This fork is probably temporary.
```
