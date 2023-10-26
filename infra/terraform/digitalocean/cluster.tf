# https://registry.terraform.io/providers/digitalocean/digitalocean/latest/docs/resources/kubernetes_cluster

resource "digitalocean_kubernetes_cluster" "talk2robots-prod" {
  name    = "k8s-do-sfo3-talk2robots-prod"
  region  = "sfo3"
  version = "1.25.4-do.0"
  node_pool {
    name       = "pool-640jpx3xu"
    size       = "s-1vcpu-2gb"
    node_count = 2
    tags       = ["talk2robots-prod"]
  }
  tags         = ["talk2robots-prod"]
  auto_upgrade = false
}

resource "digitalocean_loadbalancer" "talk2robots-lb-prod" {
  name   = "aa82b334f017a4f2497526900b2c5bba"
  region = "sfo3"
  forwarding_rule {
    entry_port      = 443
    entry_protocol  = "tcp"
    target_port     = 30216
    target_protocol = "tcp"
    tls_passthrough = false
  }

  forwarding_rule {
    entry_port      = 80
    entry_protocol  = "tcp"
    target_port     = 31681
    target_protocol = "tcp"
    tls_passthrough = false
  }
}
