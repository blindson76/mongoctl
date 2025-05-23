ports {
  http = 4246
  rpc  = 4247
  serf = 4248
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