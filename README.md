#  LastChance Messenger

> *"Because privacy shouldn't be your last resort, but your first choice."*

[![License: AGPL v3](https://img.shields.io/badge/License-AGPLv3-blue.svg)](https://www.gnu.org/licenses/agpl-3.0)
[![Go Version](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go)](https://go.dev)
[![Wails](https://img.shields.io/badge/Wails-v2-4B32C3?logo=wails)](https://wails.io)
[![React](https://img.shields.io/badge/React-18-61DAFB?logo=react)](https://reactjs.org)
[![Security: Hardened](https://img.shields.io/badge/Security-Hardened-red)](./SECURITY.md)

**LastChance Messenger** is a hardened, security-focused P2P communication terminal designed for high-assurance environments. Built using **Go (Wails)** and **React**, it provides a deterministic, decentralized messaging experience that prioritizes cryptographic integrity and metadata obfuscation. Unlike traditional messengers, LastChance operates over standard TCP/IP while maintaining a *"Red Team"* posture against traffic analysis and interception.

---

##  Key Features

| Category | Implementation |
|----------|----------------|
| **Asymmetric E2EE** | X25519 (ECDH) key exchange + XChaCha20-Poly1305 AEAD |
| **SafeFSOps (Sandbox)** | CWE-22 path traversal prevention via centralized resolution layer |
| **Stealth Traffic Masking** | Customizable headers (e.g., `X-DNS-Cookie`) to bypass DPI |
| **BIP39 Identity** | 24-word seed phrase for portable, recoverable identities |
| **Hardened Local Storage** | `0600` permissions + SQLite WAL mode for identity & messages |

---

##  Technical Architecture

### Dual-Layer Encryption (Defense in Depth)

| Layer | Protocol | Purpose |
|-------|----------|---------|
| **Outer (Transport)** | TLS 1.3 (enforced `tls.VersionTLS13`) | Anti-sniffing, local network protection |
| **Inner (Payload)** | HKDF-SHA256 + X25519 session secret | Post-TLS termination protection |

### Packet Padding Logic (Naive Secure)

Every message is padded to the **nearest allowed size**: `256B`, `1KB`, `4KB`, or `64KB`.

- **Magic Bytes**: `0x4E53` (`"NS"` - Naive Secure)
- **Entropy**: Random padding before encryption ensures identical messages produce different ciphertexts.

---

##  Security Foundations

The codebase has undergone a **comprehensive pre-commit security review**:

| CWE | Vulnerability | Remediation |
|-----|---------------|--------------|
| CWE-22 | Path Traversal | `SafeFSOps` sandbox with mandatory resolution |
| CWE-190 | Integer Overflow | Bounds checking on all size conversions |
| Deprecated Crypto | Weak primitives | Migrated to `crypto/ecdh` (Go stdlib) for X25519 |
| Resource Leaks | Handles/rows | Strict `defer` patterns for cleanup |

---

##  Installation & Quick Start

### Prerequisites

- Go **1.26+**
- Node.js **18+** & NPM
- Wails CLI  
  ```bash
  go install github.com/wailsapp/wails/v2/cmd/wails@latest
  ```
  
## Setup

### Clone the repository
  ```bash
  git clone https://github.com/yourorg/lastchance-messenger.git
  cd lastchance-messenger
  ```
### Configure Environment
  Copy .env.example to .env and adjust relay server URLs.

### Build the application
  ```bash
  wails build
  ```
    
 The binary will be located in build/bin/.

### Configuration (.env)
  | Variable |	Description	| Default |
  |--------------|----------|----------|
  | SERVER_URL	Base URL | for message relay server |	https://relay.lastchance.example |
  | LC_MASK_HEADER_NAME |	Custom header for traffic masking |	X-DNS-Cookie |
  | LC_MASK_HEADER_VALUE |	Header value for masking | MySecretToken123 |


## Documentation & Transparency

We believe in radical transparency for security tools.

#### [Technical API Specifications](https://github.com/NablaShell/LastChance/blob/main/docs/API.md) — Detailed package & encryption logic

#### [Security Audit Report](https://github.com/NablaShell/LastChance/blob/main/docs/Security_Audit_%26_Remediation_Report.md) — Full vulnerability log & fixes

## Future Roadmap

- [x] Private nodes — Released a dedicated repository with a server node in Dockerfile format. [Deploy here](https://github.com/NablaShell/LastChance-server)

- [ ] Availability — Add publicly accessible nodes for communication without private nodes

- [ ] Mesh Networking — Direct peer discovery without relay servers

- [ ] Mobile Clients — React Native port of core logic

- [ ] Post-Quantum Cryptography — Kyber/Dilithium integration

- [ ] Tor/I2P Integration — Optional anonymization network routing##  Contact & Connectivity

## Contact & Connectivity

I prefer decentralized and encrypted communication channels.

*   **Session ID**: 05f08d7242fe9cd621e98ef902cd1a21a8bf10d0c7c946e8c8e469d2396657a637
> (Preferred for quick chats)
*   **Proton Mail**: `nabla.shell@proton.me` (For long-form inquiries; PGP preferred)
*   **PGP Key**: Available in [here](/docs/public_key.asc)
    *PGP Fingerprint: 885F 3675 1D87 3F99 55ED 0ABC D1F6 A559 1458 507D*

## Funding

If LastChance helps your OpSec, consider supporting the project.
Cryptocurrency	Address

## Support the Project 

If you find **LastChance** useful, consider supporting its development:

| Asset | Address |
| :--- | :--- |
| **BTC** | 8Arc4tRdGAKcWNMLCb7mj2fnYqWgQGhTTgR7FEGaZpL2Pw6MNSwqsGMUGpeQGURgQbDoyxU1ASKMP7dKBJq8yJgCSwCgPYe |
| **XMR** | bc1qktffxm3579v6zs6mpms4yvwp6m067nkggd8ach |

*All donations go towards relay nodes and security audits.*

### License

LastChance Messenger is released under the GNU Affero General Public License v3.
You may copy, distribute, and modify the software as long as your modifications are also made available under the AGPLv3.
