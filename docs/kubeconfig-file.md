# Local Kubeconfig from k3s

A short guide to pulling the kubeconfig out of a k3s cluster so `kubectl` or this app can use
it from another machine.

k3s writes its kubeconfig pointing at `https://127.0.0.1:6443`, so that address only works
from the k3s node itself. Reaching the API server through the node's IP or hostname fails TLS
verification, because that address is not in the API server certificate's SAN list yet. That
is what `tls-san` is for.

## 1. Add `tls-san` to the k3s config

Create or edit the k3s server config file (default: `/etc/rancher/k3s/config.yaml`):

```yaml
# /etc/rancher/k3s/config.yaml
tls-san:
  - 192.168.1.10                # node IP
  - k3s.local                   # hostname, optional
write-kubeconfig-mode: "0644"   # optional: readable without sudo
```

- List the IP/hostname you will actually connect with, not `127.0.0.1`.
- If your install keeps its config elsewhere (for example `/etc/k3s/`), adjust the path. The
  file name and the flags themselves stay the same.

## 2. Restart k3s

```bash
sudo systemctl restart k3s
```

The API server certificate is regenerated so it includes the new SANs.

## 3. Copy the kubeconfig

```bash
sudo cp /etc/rancher/k3s/k3s.yaml ~/.kube/config
sudo chown "$USER" ~/.kube/config
chmod 600 ~/.kube/config
```

Then change `server:` from `127.0.0.1` to the node address, which must match one of the
`tls-san` entries:

```yaml
server: https://192.168.1.10:6443
```

The `certificate-authority-data`, `client-certificate-data` and `client-key-data` fields stay
untouched.

## 4. Point the app at it

This app resolves the kubeconfig in one order: a manual pick through the **Choose kubeconfig**
button, then `$KUBECONFIG`, then `~/.kube/config`, then
`<project dir>/.kube/config`. Dropping the file at `~/.kube/config` is therefore enough, or
you can select it straight from the dialog.

## Verify

```bash
kubectl --kubeconfig ~/.kube/config cluster-info

openssl s_client -connect 192.168.1.10:6443 </dev/null 2>/dev/null \
  | openssl x509 -noout -text | grep -A1 "Subject Alternative Name"
```

The second command prints the SAN list. Make sure the IP/hostname you connect with appears
there.

## Notes

- A kubeconfig holds cluster admin credentials, so never commit it. This repo's `.gitignore`
  already ignores `.kube/` and `kubeconfig`.
- If the app runs on the same machine as the k3s node, `https://127.0.0.1:6443` is enough and
  `tls-san` is not needed.
