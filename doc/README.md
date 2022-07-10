Experimental I2P-Enhanced rdsys fork
====================================

This is a fork of `rdsys` which enables it to use I2P as a way of distributing 
resources to I2P users. In a minor way, it's the first project to attempt to
extend rdsys in a way which tightly integrates it with a non-Tor Network. In
that way it's intended to help prove, or disprove the hypothesis that selective,
intelligent forms of network mixing can help Internet Freedom projects help
eachother. Using this system, I2P users can cooperate with Tor users to provide
bridges and help users access the Tor network, and rdsys service operators can
provide access to reseeds and Tor Browsers to I2P users.

Why a fork?
-----------

These features needed to be tested together, but probably should be merged upstream
incrementally. I think it will be desirable to upstream some of these features to
`rdsys` itself soon. It is modified to do three things so far:

 1. Listen on an I2P address. I2P addresses are A) Cryptographically secured
 and B) reached by multiple hops which are agnostic of the origin and nature
 of of the content they are routing. I2P users can leverage this reachability
 to acquire a Tor bridge when they need to access `.onion` services or the
 non-anonymous parts of the web.
 2. Distribute I2P Bridges. I2P is able to set up listeners and connections
 which can act as proxies to unlisted Tor relays via the Pluggable Transports
 mechanism. These bridges sacrifice some performance by taking 2 additional
 hops through the I2P network, in order to gain resistance to enumeration when
 a malicious client connects with them. This works just like the HTTPS distributor
 for simplicity's sake, but it could use email-over-I2P or a more sophisticated
 distributor in the future.
 3. Distribute Tor Browser via I2P Magnet Links. This allows users of Tor Browser
 who also participate in I2P Torrents to update using I2P's anonymous peer-to-peer
 capabilities. I2P Torrents, like all torrents, are downloaded from multiple peers
 and are downloaded out-of-order, and torrent downloads are self-verifying.
 Actually accomplishing this requires both changes to `rdsys` and the implementation
 of an I2P Torrent client for actually downloading the bundle and seeding it to
 peers.

Moreover, these are just the integrations that I2P can offer. The larger point is
that Internet Freedom projects sometimes have places where they *expressly* interface
with other communities. I2P has API's which it offers to enable people to develop
software by automating how it builds connections. Tor has infrastructure tools like
rdsys where it leverages other Internet Freedom projects to provide it's users with
valuable services. All I have done here, really, is connect the former with the latter.
I urge you to think about how other software might be able to not only provide something
useful to a particular project but to participate in a community of Privacy-Enhancing
Technologies and Internet Freedom development by building bridges between tools at
critical points.

That's really just the beginning though. In the future, it may be desirable to:

 1. Extend this fork to distribute links to I2P Reseed bundles which I2P uses to
 bootstrap. This is essentially the same as distributing gettor links except we
 should not use services like github or gitlab, where releases are linked to git
 checkins where the binary content may be permanently retrievable. Since that's
 the case, email, telegram, and potentially some sort of proxy would be better
 systems to use. This would eliminate any need for `reseed-tools` to reproduce
 functionality to send reseed bundles using any of these messaging services.
 2. Extend this fork to sometimes just hand out a single routerInfo from a local
 NetDB, which is not much longer than a link and enough to start bootstrapping
 an I2P router. By wrapping a routerInfo in an `.su3` to create a mini-reseed
 and encoding the result as base64 string, we can transmit a routerInfo in a short
 encrypted message or by any other means which can be built into rdsys.
 3. Allow and encourage I2P users to run Tor Bridges for eachother and for people
 who choose to use I2P and Tor at the same time for different purposes. This would
 allow them to forward traffic to Tor for eachother anonymously while remaining
 agnostic of the traffic itself, making it appropriate for more types of organizations
 to operate without the risks of traditional outproxies. It would also reduce strain
 on the outproxies and provide a path to obfuscate Tor traffic and leverage I2P's
 relay diversity to reach the bridge.

It's not a coincidence that most of these points where Tor and I2P can interface with
eachother correspond with getting people on the network. Blocking the very first
connection made by an I2P router is relatively easy, and cutting down Tor connections
by blocking client connections to known relays is also relatively easy. Blocking our
public download locations is also pretty easy. All of these relate to what might be
termed a "Bootstrap Problem" where the network is far easier to block when the user
starts using it as opposed to when a user has already been connected.

Once you're connected, both I2P and Tor have options to help you stay connected, but
more importantly the different networks experience slightly different versions of the
bootstrap problems, with different weaknesses. The hypothesis is that if one is blocked,
and not another, then sometimes, one tool can be used in practice to resolve the
bootstrap issues experienced by another. An example of how this already happens would be
using Telegram to hand out bridge URL's or Tor Browser download links. This fork just
extends that concept to I2P.
