# ISHOCON3 Manual

This document explains specific details about the competition, including how to start the reference implementation application, how to run benchmarks, and server configuration for ISHOCON3.


## Running the Application

The method to start the application differs depending on which language's reference implementation you use.

Starting the Python implementation:

```
$ cd ~/webapp/python
$ uv run gunicorn -c gunicorn.conf.py --bind "0.0.0.0:8080"
```

Starting the Ruby implementation:

```
$ cd ~/webapp/ruby
$ bundle exec puma -b tcp://0.0.0.0:8080
```


## Running the Benchmark

There is an executable file named `benchmark` in the root directory, which you can use to run benchmarks.

```
$ cd ~/
$ ./benchmark
```

The benchmark supports 4 log levels, which can be specified with the `--log-level` option.

```
$ ./benchmark --log-level info
```

Valid values are `debug`, `info`, `warn`, `error`, with `info` as the default.
* `debug`: When you want to output all requests and user actions from the benchmark
* `info`: When you want to output user actions and events
* `warn`: When you want to output only warning messages that could prevent score improvement
* `error`: When you want to output only error messages


If you want to trace the behavior of specific users or administrators, you can do so as follows:

```
$ ./benchmark --log-level debug > debug.log 2>&1
$ cat debug.log | grep "user=user12345"
$ cat debug.log | grep "user=admin"
```

### Benchmark Execution Flow

The load test is executed as follows:

1. `POST /api/initialize` (timeout in 10 seconds)
1. Load test and application integrity check (60 seconds)
1. Final check (several seconds to tens of seconds)

If the initialization process or application integrity check fails, the load test will immediately fail.

HTTP response processing is terminated at the end of the load test. Incomplete requests will be forcibly disconnected. Requests forcibly disconnected after the load test ends do not affect the score or verification.


### Timeouts

The timeouts for requests from the benchmark are set as follows:

| Request | Timeout |
|---------|---------|
| GET /api/admin/stats | 2 seconds |
| GET /api/admin/train_sales | 2 seconds |
| All other requests | 10 seconds |


### Benchmark Termination

The benchmark execution will be immediately terminated in the following situations. When terminated, the score at that point becomes the final score.

* An error or timeout occurs during initialization
* An API response related to administrator actions times out
* An API related to administrator actions returns an error
* Application integrity check fails


### Grace Period

A delay of 1 second is allowed for the sales data returned by `GET /api/admin/stats` and `GET /api/admin/train_sales`. In other words, the data returned by these APIs is considered correct even if it is data from up to 1 second ago.

If the values are not correct, it violates the application integrity check and the benchmark will fail.


## Server Configuration

The server specifications are as follows:

- **Instance Type**: c7i.xlarge (4 vCPU, 8 GB Mem)
- **Volume**: 8GB, General Purpose SSD (gp3)

### MySQL

MySQL (8.0) is running on port 3306. The initial users are as follows:

- Username: `root`, Password: `ishocon`
- Username: `ishocon`, Password: `ishocon`

You can switch to a different MySQL version or use a different database if needed.

Running `~/webapp/sql/init.sh` will initialize the database. This is the same process that `POST /api/initialize` executes.

If you want to add indexes or modify the schema, it is recommended to either modify the existing `*.sql` files or create new SQL files and add them to `init.sh`.

### Nginx

Nginx (1.24) is running on port 80. You can change the configuration if needed.

Static files are located in `~/webapp/public/*`, and when accessing the site from a browser, these files are loaded to allow you to verify the API behavior.

The competition target is only the backend API, and these static files must not be modified.
Also, the benchmark does not access these static files.

### Payment App

The payment service has an executable file located in `~/payment_app` and is running as a systemd service on port 8081.

```
sudo systemctl status payment_app
```

The payment service is accessed by the application during user payment and returns payment success or failure based on the user's credit information.

The payment service data is initialized at the `POST /initialize` endpoint, and in the reference implementation, this endpoint is called from the application's `POST /api/initialize`.

This service is not subject to optimization and cannot be modified.
