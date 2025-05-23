ports {
  http = 4446
  rpc  = 4447
  serf = 4448
}
bind_addr = "10.10.11.1"

server {
    enabled = true
}

client {
    enabled = true
}
plugin "raw_exec" {
  config {
    enabled = true
  }
}