# SuperQueueRequestRouter

A simple PoC of making a request router to handle horizontal scalability of SuperQueues by randomly serving POST and GET /record requests to partitions, then during a POST /ack or POST /nack returning the request to the proper partition.

The goal is completely linear horizontal scalability.
