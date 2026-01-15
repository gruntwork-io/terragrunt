# Reporting Security Issues

Gruntwork takes security seriously, and we value the input of independent security researchers. If you're reading this because you're looking to engage in responsible disclosure of a security vulnerability, we want to start with thanking you for your efforts. We appreciate your work and will make every effort to acknowledge your contributions.

To report a security issue, please use the GitHub Security Advisory ["Report a vulnerability"](https://github.com/gruntwork-io/terragrunt/security/advisories/new) button in the ["Security"](https://github.com/gruntwork-io/terragrunt/security) tab.

After receiving the report, we will investigate the issue and inform you of next steps. After the initial reply, we may ask for additional information, and will endeavor to keep you informed of our progress.

If you are reporting a bug related to an associated tool that Terragrunt integrates with, we ask that you report the issue directly to the maintainers of that tool.

Please do not disclose the issue publicly until we have had a chance to address it.

## Expectations on timelines

You can expect that Gruntwork will take any report of a security vulnerability seriously, but we ask that you also respect that it can take time to investigate and address issues given the size of the team maintaining Terragrunt. We will do our best to keep you informed of our progress, and provide insight into the timeline for addressing the issue.

## Thank you

We appreciate your help in making Terragrunt more secure. Thank you for your efforts in responsibly disclosing security issues, and for your patience as we work to address them.

## Verifying Release Signatures

All Terragrunt releases are signed with both GPG and Cosign. You can verify the authenticity of downloaded binaries using either method.

### Download Verification Files

```bash
VERSION="v0.XX.X"  # Replace with actual version
curl -LO "https://github.com/gruntwork-io/terragrunt/releases/download/${VERSION}/SHA256SUMS"
curl -LO "https://github.com/gruntwork-io/terragrunt/releases/download/${VERSION}/SHA256SUMS.gpgsig"
curl -LO "https://github.com/gruntwork-io/terragrunt/releases/download/${VERSION}/SHA256SUMS.sig"
curl -LO "https://github.com/gruntwork-io/terragrunt/releases/download/${VERSION}/SHA256SUMS.pem"
```

### GPG Verification

```bash
# Import the public key (first time only)
curl -s https://gruntwork.io/.well-known/pgp-key.txt | gpg --import

# Verify the signature
gpg --verify SHA256SUMS.gpgsig SHA256SUMS

# Verify binary checksum
sha256sum -c SHA256SUMS --ignore-missing
```

### Cosign Verification

```bash
# Install cosign: https://docs.sigstore.dev/cosign/system_config/installation/
cosign verify-blob SHA256SUMS \
  --signature SHA256SUMS.sig \
  --certificate SHA256SUMS.pem \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp "github.com/gruntwork-io/terragrunt"

# Verify binary checksum
sha256sum -c SHA256SUMS --ignore-missing
```
