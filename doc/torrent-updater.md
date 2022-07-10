A Torrent-Based Updater for Tor Browser Bundle
==============================================

Having a torrent-based updater for Tor Browser Bundle might be useful
because it enables downloads which do not depend on a single service
or static mirrors, but rather relies on users across the spectrum of
Tor Browser users who are willing to share it to distribute it to many
others. As we all know, just using Tor does not hide that you are using
Tor, any network observer can discover that. Since this is the case,
for many users of so-called "Vanilla" Tor it may be seen as prosocial
and desirable to participate in sharing the Tor Browser Bundle by
contributing their own bandwidth. Combined with WebTorrent this could
provide a sort of "Donor CDN" of concerned netizens helping keep Tor
Browser Bundle downloads alive.

However, since Bittorrent connections are normally *direct* peer-to-peer
connections, this may reveal information about the user to other people
participating in the torrent swarm. This may not be desirable. So, as a
first step, I have implemented torrents which have I2P-only trackers which
are DHT-supported. So the torrents will begin between I2P participants
who's peer-to-peer connections will be garlic-routed through the I2P
network in order to prevent them leaking information. Then "Bridged"
participants in I2P torrents can help seed them to the rest of the non-I2P
Bittorrent network, keeping the disclosure of sharing metadata voluntary
throughout the whole process.

needsUpdate for a Torrent-Based Updater
---------------------------------------

TODO: Describe unique needs(and simplicity) of this function for a
Torrent-based updater

newRelease for a Torrent-Based Updater
--------------------------------------

TODO: Describe unique needs(and complexity) of this function for a
Torrent-based updater

Clients for a Torrent-Based Updater
-----------------------------------

Of course, this requires a Torrent client to use, and in ideal circumstances,
an I2P-enabled Torrent client. I2P is used to bootstrap the torrent without
revealing the location of the initial seeders, and will for the time being
updates will be "Visible" faster when they are performed within I2P. For now,
I enable this using a hack of the built-in I2P Java torrent client, which is
called "I2PSnark." In the future, a pure-Go bittorrent-over-I2P client would
be better. Of course, that would still require an I2P router to be running on
the same host as the Tor Browser. In it's most useful form, it would also embed
a pure-Go I2P router for the pure-Go I2P torrent client to use.
