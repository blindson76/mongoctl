ports {
  http = 4546
  rpc  = 4547
  serf = 4548
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