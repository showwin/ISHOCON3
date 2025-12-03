
# Local dev

## Prerequisites

* Install [Task](https://taskfile.dev/)


## Run the Python version

```bash
task python:build
task python:up
task initialize
task benchmark
```

To follow logs:

```bash
task python -- logs -f webapp
```

To shut down:

```bash
task python:down
```


## Run the Ruby version

```bash
task ruby:build
task ruby:up
task initialize
task benchmark
```

Follow logs:

```bash
task ruby -- logs -f webapp
```

Shut down:

```bash
task ruby:down
```

## Run benchmark

```
task benchmark
```
