# Telegraf Input Plugin for Acaia Coffee Scales

[![Acaia metrics on Grafana dashboard](https://img.youtube.com/vi/JSJyuh2IVqk/0.jpg)](https://www.youtube.com/watch?v=JSJyuh2IVqk)

That's a fork of [Telefraf](https://github.com/influxdata/telegraf/) with [Acaia input](plugins/inputs/acaia) built-in statically.

## Configuration

```toml
[agent]
#interval = "1s"
flush_interval = "500ms"
omit_hostname = true
quiet = true
#debug = true

[[inputs.acaia]]
model = "ACAIAL1"

[[inputs.acaia]]
model = "PEARLS"

[[outputs.file]]
files = ["stdout"]
data_format = "influx"

[[outputs.influxdb]]
urls = ["http://127.0.0.1:8086"]
database = "acaia"

```
