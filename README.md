#  LastChance Messenger

> *"Because privacy shouldn't be your last resort, but your first choice."*

[![License: AGPL v3](https://img.shields.io/badge/License-AGPLv3-blue.svg)](https://www.gnu.org/licenses/agpl-3.0)
[![Go Version](https://img.shields.io/badge/Go-1.21%2B-00ADD8?logo=go)](https://go.dev)
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

- Go **1.21+**
- Node.js **18+** & NPM
- Wails CLI  
  ```bash
  go install github.com/wailsapp/wails/v2/cmd/wails@latest
