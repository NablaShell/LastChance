Technical Documentation: LastChance P2P Messenger
1. Package: crypto
Purpose: Provides cryptographic primitives for end-to-end encryption (E2EE), key exchange, and message integrity. It ensures that all data (messages and files) remains confidential and authenticated.
Key Structures and Methods:

    Constants: AllowedTotalSizes (256, 1024, 4096, 65536 bytes) for packet padding to prevent traffic analysis.
    GenerateSharedKey(myPrivateKey, theirPublicKey []byte): Performs X25519 ECDH key exchange followed by HKDF-SHA256 to derive a 32-byte shared secret.
    EncryptMessage(plaintext, sharedSecret []byte, sessionID uint64): Encrypts data into a fixed-size packet.
    DecryptMessage(packet, sharedSecret []byte): Validates packet size and decrypts the content, returning the plaintext and session ID.
    EncryptFile / DecryptFile: Specifically for file data, using shared secrets without padding.
    SignEd25519 / VerifyEd25519: Methods for generating and checking digital signatures.

Encryption Algorithms:

    Key Exchange: X25519 (via crypto/ecdh).
    Symmetric Encryption: The codebase uses XChaCha20-Poly1305 (evidenced by NonceSize = 24) for message and file encryption.
    Authentication/Signatures: Ed25519 for server-side authorization and identity verification.
    Hashing: SHA256 for identity hashes and key derivation.


--------------------------------------------------------------------------------
2. Package: network
Purpose: Manages communication with the remote servers, including message polling (PULL), message delivery (PUSH), and file transfers.
Key Structures and Methods:

    Sender: Handles outgoing HTTP requests for sending messages and uploading files.
    Listener: Manages background polling of the message queue with automatic rate-limit adaptation.
    PullMessagesWithAuth(identityHash, pubKeyHex, privKey): Retrieves messages from the server using Ed25519-signed timestamps for authentication.
    UploadFile(payload, fileName): Uploads encrypted file blobs to the file server.

API Endpoints & Client Logic:

    PUSH Messages: POST {ServerBaseURL}/push/{targetHash}. Sends encrypted packets. Supports exponential backoff for 429 Rate Limit errors.
    PULL Messages: GET {ServerBaseURL}/pull/{identityHash}.
        Logic: Requires headers: X-Timestamp, X-Public-Key, and X-Signature.
        Signature: Ed25519.Sign(privKey, timestamp + identityHash).
    UPLOAD File: POST {FileServerURL}/upload. Returns a SHA256 hash of the file.
    DOWNLOAD File: GET {FileServerURL}/download/{fileHash}.

Traffic Masking: The client uses custom headers to disguise traffic as standard web requests. Configuration can be pulled from environment variables:

    LC_MASK_HEADER_NAME: e.g., X-DNS-Cookie or X-Requested-With.
    LC_MASK_HEADER_VALUE: Custom hash or XMLHttpRequest.


--------------------------------------------------------------------------------
3. Package: storage
Purpose: Handles local data persistence for contacts, message history, and file records using an SQLite database and a sandboxed file system.
Key Structures and Methods:

    Storage: Main structure managing the SQLite connection.
    Contact / Message / FileRecord: Data models for DB entities.
    SafeFSOps: A security layer providing CWE-22 (Path Traversal) protection by sandboxing all file operations within a root directory.
    AddContact(hash, publicKey, nickname): Persists or updates contact information.
    SaveMessageWithRoom(contactHash, direction, roomHash, text): Stores chat history.

Storage Security:

    Uses WAL mode for concurrent access.
    Implements 0600 (owner-only) file permissions for sensitive data like identity files and downloads.


--------------------------------------------------------------------------------
4. Package: identity
Purpose: Manages the user's cryptographic "brain," including key generation from seed phrases and identity persistence.
Key Structures and Methods:

    Identity: Stores Nickname, SeedPhrase, X25519 keys, Ed25519 keys, and the unique Hash.
    IdentityManager: Handles loading/saving the identity.json file.
    deriveKeysFromSeed(seed): Uses HKDF to derive separate keys for encryption (X25519) and authentication (Ed25519) from a single master seed.
    ComputeIdentityHash(pubKey): Generates the public identityHash using SHA256(Ed25519_PublicKey).

Logic: The identity is the root of trust. The identityHash serves as the user's address on the network. For every message poll, the identity package provides the necessary keys to sign the request, proving ownership of the hash without revealing private keys.
