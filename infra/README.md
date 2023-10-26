# DigitalOcean cheatsheet

```bash
alias tf=terraform
cd terraform/digitalocean
doctl auth init
tf init
tf import -var "do_token=${DO_PAT}" digitalocean_kubernetes_cluster.talk2robots-prod {your cluster id}
tf import -var "do_token=${DO_PAT}" digitalocean_loadbalancer.talk2robots-lb-prod {your load balancer id}
tf plan -var "do_token=${DO_PAT}"
```
