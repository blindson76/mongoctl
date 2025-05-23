ports {
  http = 4646
  rpc  = 4647
  serf = 4648
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