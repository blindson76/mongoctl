ports {
  http = 4146
  rpc  = 4147
  serf = 4148
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