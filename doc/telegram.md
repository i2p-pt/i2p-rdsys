Telegram distributor
====================

The telegram distributor uses Telegram as mechanism to distribute resources. It 
uses Telegram's bot API to interact with Telegram and to respond to resource 
requests.

It uses the telegram user id as an indicator of the age of the requesting 
account. The distributor split the telegram accounts in two groups: *old* and 
*new*. There is a configuration parameter `min_user_id` where accounts with 
lower user id than `min_user_id` are considered *old* and accounts with higher 
user id are considered *new*.

For *old* accounts the distributor uses resources from the rdsys backend. For 
*new* accounts the distributor uses resources from a provided file 
(`new_bridges_file` in the configuration), so those can be rotated when found 
being blocked.

Each account will get the same resources for a period of time configured in 
`rotation_period_hours`.
