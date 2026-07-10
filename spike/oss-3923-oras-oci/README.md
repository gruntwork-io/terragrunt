# Spike: oras-go/v2 vs the OpenTofu OCI module contract

Throwaway spike code. NOT production code, never merged to main. Results in `FINDINGS.md`.

## Reproduce

```bash
# 1. Self-signed TLS cert (OpenTofu refuses plain-HTTP registries, even localhost)
mkdir -p certs && openssl req -x509 -newkey rsa:2048 -nodes -keyout certs/registry.key -out certs/registry.crt -days 7 -subj "/CN=localhost" -addext "subjectAltName=DNS:localhost,IP:127.0.0.1"

# 2. Local OCI registry (distribution v3) with TLS
docker run -d --name spike-registry -p 127.0.0.1:5000:5000 -v $PWD/certs:/certs:ro -e REGISTRY_HTTP_TLS_CERTIFICATE=/certs/registry.crt -e REGISTRY_HTTP_TLS_KEY=/certs/registry.key registry:3

# 3. Publish a module zip the OpenTofu way (oras CLI)
zip -r module.zip main.tf
oras push --ca-file certs/registry.crt 127.0.0.1:5000/terraform-modules/vpc:1.0.0 --artifact-type application/vnd.opentofu.modulepkg module.zip:archive/zip

# 4. Consume with OpenTofu (behavioral oracle)
#    module "m" { source = "oci://127.0.0.1:5000/terraform-modules/vpc?tag=1.0.0" }
SSL_CERT_FILE=$PWD/certs/registry.crt tofu init

# 5. Consume with the spike (clean-room oras-go v2.5.0), then diff the trees
go build .
./spike -ca certs/registry.crt -ref 1.0.0 -dst ./out
diff -r ./out <tofu-dir>/.terraform/modules/m

# Extras
./spike -ca certs/registry.crt -errdef-check     # errdef.ErrNotFound classification
./spike -ambient-order ...                       # OpenTofu-order multi-path ambient credentials
```
