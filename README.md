LastChance Messenger
"Because privacy shouldn't be your last resort, but your first choice."
LastChance Messenger is a hardened, security-focused P2P communication terminal designed for high-assurance environments. Built using Go (Wails) and React, it provides a deterministic, decentralized messaging experience that prioritizes cryptographic integrity and metadata obfuscation. Unlike traditional messengers, LastChance operates over standard TCP/IP while maintaining a "Red Team" posture against traffic analysis and interception.

--------------------------------------------------------------------------------
⚡ Key Features

    Asymmetric E2EE: Industry-standard end-to-end encryption using X25519 (ECDH) for key exchange and XChaCha20-Poly1305 for AEAD message security.
    SafeFSOps (Sandbox): A centralized path-resolution layer that prevents CWE-22 (Path Traversal) by restricting all filesystem operations to a dedicated, sanitized storage root.
    Stealth Traffic Masking: Disguises P2P traffic using customizable "Stealth Headers" (e.g., X-DNS-Cookie or X-Requested-With) to bypass deep packet inspection (DPI).
    BIP39 Identity: Deterministic identity generation from a 24-word seed phrase, ensuring portability and cryptographic recovery.
    Hardened Local Storage: Sensitive data, including the identity.json and message databases, are protected with 0600 (owner-only) permissions and SQLite WAL mode.


--------------------------------------------------------------------------------
🏗 Technical Architecture
Dual-Layer Encryption
LastChance implements a "defense-in-depth" networking model:

    Outer Layer (Transport): All external traffic is wrapped in TLS 1.3 (enforced via MinVersion: tls.VersionTLS13), protecting against local network sniffing.
    Inner Layer (Payload): Even if the TLS layer is terminated or intercepted, the underlying payload is encrypted with a unique session secret derived via HKDF-SHA256 from a X25519 shared key.

Packet Padding Logic (Naive Secure)
To prevent side-channel attacks based on packet size analysis, LastChance uses a grid-based padding system. Every message is padded to the nearest allowed size: 256B, 1KB, 4KB, or 64KB.

    Magic Bytes: All encrypted frames begin with the 0x4E53 ("NS" - Naive Secure) identifier.
    Entropy: Random padding is appended to the payload before encryption to ensure that even identical messages result in entirely different ciphertexts.


--------------------------------------------------------------------------------
🛡 Security Foundations
The codebase has undergone a comprehensive pre-commit security review to eliminate common vulnerabilities:

    CWE-22 (Path Traversal): Remediated via a mandatory SafeFSOps sandbox.
    CWE-190 (Integer Overflow): Bounds checking implemented for all file and packet size conversions.
    Cryptographic Hygiene: Migrated from deprecated primitives to the modern crypto/ecdh Go standard library for X25519 operations.
    Resource Management: Strict patterns for closing file handles and database rows to prevent memory leaks and descriptor exhaustion.


--------------------------------------------------------------------------------
🚀 Installation & Quick Start
Prerequisites

    Go (1.21+)
    Node.js (18+) & NPM
    Wails CLI (go install github.com/wailsapp/wails/v2/cmd/wails@latest)

Setup

    Clone the repository:
    Configure Environment: Copy the example configuration and adjust your server URLs.
    Build the Application: Generate a production-ready redistributable package.
    The binary will be located in the build/bin directory.


--------------------------------------------------------------------------------
⚙ Configuration
The application behavior is controlled via .env flags:

    SERVER_URL: The base URL for the message relay server.
    LC_MASK_HEADER_NAME: The custom header used to mask traffic (default: X-DNS-Cookie).
    LC_MASK_HEADER_VALUE: The value for the masking header.


--------------------------------------------------------------------------------
📖 Documentation & Transparency
We believe in radical transparency for security tools. Please review our technical specifications and audit results:

    Technical API Specs:  — Detailed breakdown of packages and encryption logic.
    Security Audit Report:  — Full log of identified vulnerabilities and their fixes.


--------------------------------------------------------------------------------
🗺 Future Roadmap

    [ ] Mesh Networking: Direct peer discovery without reliance on relay servers.
    [ ] Mobile Clients: Porting the core logic to React Native.
    [ ] Post-Quantum Cryptography: Integration of Kyber/Dilithium algorithms.
    [ ] Tor/I2P Integration: Optional routing through anonymization networks.


--------------------------------------------------------------------------------
💰 Funding
If you find this tool useful for your OpSec, consider supporting the project.

    Monero (XMR): 48...[Your_XMR_Address]
    Bitcoin (BTC): bc1...[Your_BTC_Address]


--------------------------------------------------------------------------------
LastChance Messenger is released under the GNU Affero General Public License v3.
