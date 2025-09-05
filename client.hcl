
client {
    gc_interval = "8h"
    gc_disk_usage_threshold = 99
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