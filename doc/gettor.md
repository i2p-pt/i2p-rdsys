Gettor distributror and updater
===============================

The gettor distributor provides links to download Tor Browser Bundle (TBB)
from uncensored providers over email. Its used by users on networks where 
the main Tor download website is being blocked, so they can send an email to 
gettor and receive a response with a link to download TBB that will not be 
blocked in their network.

It's implemented as a resource in the backend, a distributor that
communicates over email and a updater that uploads the new TBB releases to 
the providers.

TBLink resource
---------------

TBLink resources are defined in rdsys backend and contain the following
metadata:

* TBB download link
* signature download link
* language (en, pt-BR, ...)
* platform (linux64, win32, osx64, ...)
* version

They are different than most other resources in rdsys as they are stored in 
disk by rdsys, not partitioned and updated by an updater process.

While most other resources are passively pulled by rdsys kraken service TBLink
resources are updated by an external process (the gettor updater) that sends
the updates to the backend over HTTP requests. They are stored as a json file
on disk.

TBLink resources are unpartitioned, so each distributor with access to them
gets the full list of resources. There is no need in gettor to assign resources 
to unique distributors.

TBB releases might be for some platforms and not others. The backend, distributors
and updater are designed to provide the latest version for each platform.

Gettor updater
--------------

The gettor updater monitors Tor Browser releases when there is a new release 
uploads it to all the supported providers (github for now) and sends the new 
links to the backend over http to be propagated to the distributors.

It does listen to the [release 
json](https://aus1.torproject.org/torbrowser/update_3/release/downloads.json) 
and get the latest version for each platform of the TBB and it's signature to 
upload to each provider.

Gettor distributor
------------------

The gettor distributor listens to incoming emails over IMAP and responds with 
links to download TBB for the requested platform and language .  It does accept 
the platform and/or language being provided in the subject or the body of the 
email.

If platform is provided the distributor will answer with a help email describing 
how to use the service. If the platform is provided but no language is it will 
provide the download links for the requested platform and *en-US* language.

There are three predefined platform aliases:
* **windows**. That will provide *win32* bundles.
* **linux**. That will provide *linux64* bundles.
* **osx**. That will provide *osx64* bundles.

Providers
---------

* **github**. Uses a single repo with a release per platform, where the files 
  are release assets.
* **gitlab**. Uses one repo per platform, the files are included in the repo.
  There current version is in the project description.
* **gdrive**. Google drive.
* **s3**. Used for internet archive. Uses a bucket per platform and version.
