ClueBot NG
==========

Proof of concept rewrite of ClueBot NG's bot component (https://github.com/cluebotng/bot).

Goals
-----

* Maintain revert outcome; specifically the pre-filtering that happens before core scoring
* Improve resource usage; specifically MySQL connection pooling, ideally removing the pre-fork model
* Improve debugging; expose metrics for health, which currently are calculated from scraping logs
* Ease development; single portable binary with only external service dependencies
* Improve throughput/missed edits; or at least be able to explain them better [T343952]

Compatibility
-------------

Not supported:

* `CBAutostalk.js` & `CBAutoedit.js` - These were used to emit messages into IRC, but haven't been used since 2018 (
  80cab4)
* `oftenvandalized.txt` - This was used to emit messages into IRC, but hasn't been used since, but haven't been used
  since 2018 (80cab4)
* `irc.wikimedia.org` - In favour of the HTTP event stream (which backs the IRC relay)

TODO
----

* Add test cases for each section of functionality
* Run side by side in production & compare decisions
* Cleanup metric production
* Cleanup logging/tracing code
