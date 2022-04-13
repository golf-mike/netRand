# netRand
Terrible network-timing-based-random-number-generator turned waitgroupsize performance data generator. 
Records timings for concurrent GET requests vs sequential requests at different waitgroup sizes.  
Collected data:
- runs
  - run_id
  - timestamp
  - waitgroup_size
  - concurrent total time
  - sequential total time
  - concurrent/sequential
- concurrent_timings
  - run_id
  - channel_position
  - duration
- sequential_timings
  - run_id
  - call_number
  - duration

A single run performs:
- 100 GET requests in goroutines with a specific waitgroup size
- 100 GET requests in a for loop
- persisting of total and inidividual request times

The number of repetitions per waitgroupsize and the range of waitgroupsizes can be controlled (compile time only) by global constants. 
DBs and binary included for convenience. This project will not be further developed.

# Why
After https://stackoverflow.com/questions/71737286/unexpected-behaviour-of-time-now-in-goroutine I was curious about the impact on performance of goroutines when using 
different waitgroup sizes.

