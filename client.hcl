
client {
    gc_interval = "8h"
    enabled = true
  options = {
    "driver.allowlist" = "raw_exec,java"
  }
}
consul {
    server_service_registration {
        enabled = true
        }
    client_service_registration {
        enabled = true
        }
    }
plugin "raw_exec" {
    config {
        enabled = true
        }
    }