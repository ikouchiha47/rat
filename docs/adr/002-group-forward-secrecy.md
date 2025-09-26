# ADR-002: Group Forward Secrecy Implementation

## Status
Proposed

## Context
We need to implement forward secrecy for group messages in our chat application. Forward secrecy ensures that even if long-term keys are compromised, past communications remain secure. This is particularly challenging in group settings where multiple participants need to encrypt/decrypt messages efficiently.

## Problem Statement
- **Group Key Management**: How to securely distribute and rotate encryption keys among group members
- **Forward Secrecy**: Ensure past messages remain secure even if current keys are compromised
- **Scalability**: Efficient encryption/decryption for groups of varying sizes
- **Member Changes**: Handle users joining/leaving groups securely
- **Performance**: Minimize computational overhead for real-time messaging

## Real-World Solutions Analysis

### 1. Signal Protocol (Double Ratchet + Sender Keys)
**How it works:**
- **Pairwise Sessions**: Each member has individual Signal sessions with every other member
- **Sender Keys**: For efficiency, sender broadcasts a single encrypted message using a "sender key"
- **Key Rotation**: Sender keys rotate frequently (every message or time-based)
- **Distribution**: New sender keys distributed via pairwise Signal sessions

**Pros:**
- Strong forward secrecy
- Proven security model
- Handles member changes well

**Cons:**
- Complex implementation
- O(n) key distribution overhead for each rotation
- Requires pairwise key exchange setup

### 2. MLS (Messaging Layer Security) - IETF Standard
**How it works:**
- **Tree Structure**: Members organized in a binary tree
- **TreeKEM**: Key derivation using tree-based key encapsulation
- **Epoch Changes**: Group state changes trigger new epoch with fresh keys
- **Path Updates**: Efficient key updates propagate through tree

**Pros:**
- IETF standard (RFC 9420)
- Efficient O(log n) key updates
- Strong security guarantees
- Designed for large groups

**Cons:**
- Very complex implementation
- Newer standard, fewer implementations
- Requires careful state management

### 3. Simplified Group Diffie-Hellman
**How it works:**
- **Group Key Agreement**: All members contribute to shared secret
- **Key Derivation**: Derive encryption keys from shared secret
- **Rotation**: Periodic re-keying with new DH contributions

**Pros:**
- Simpler to implement
- Well-understood cryptography
- Good for smaller groups

**Cons:**
- Limited forward secrecy
- Vulnerable to member compromise
- Inefficient for large groups

## Decision

We will implement a **hybrid approach** inspired by Signal's Sender Keys but simplified for our use case:

### Architecture: "Rotating Group Keys with Diffie-Hellman Bootstrap"

#### Phase 1: Initial Key Exchange
1. **Group Formation**: When a room is created, perform group DH key exchange
2. **Shared Secret**: All members contribute to establish initial group secret
3. **Key Derivation**: Derive initial encryption key from group secret

#### Phase 2: Forward Secrecy via Key Rotation
1. **Sender Keys**: Each message sender generates a new ephemeral key
2. **Key Wrapping**: Sender encrypts message with ephemeral key
3. **Key Distribution**: Ephemeral key encrypted with current group key
4. **Ratcheting**: Group key rotates based on message count or time

#### Phase 3: Member Management
1. **Join**: New member gets current group key via DH exchange with existing member
2. **Leave**: Trigger immediate group re-keying to exclude departed member
3. **Compromise Recovery**: Manual re-keying command available

## Implementation Plan

### Cryptographic Primitives
- **Key Exchange**: X25519 (Curve25519 ECDH)
- **Symmetric Encryption**: ChaCha20-Poly1305
- **Key Derivation**: HKDF-SHA256
- **Random Generation**: crypto/rand

### Message Format
```json
{
  "type": "encrypted_message",
  "sender": "alice",
  "room": "general",
  "encrypted_content": "base64_encrypted_data",
  "ephemeral_key": "base64_wrapped_ephemeral_key",
  "key_id": "current_group_key_id",
  "timestamp": "2025-01-01T00:00:00Z"
}
```

### Key Management Structure
```go
type GroupKeyManager struct {
    RoomID          string
    CurrentKeyID    string
    GroupKey        []byte
    EphemeralKeys   map[string][]byte  // message_id -> ephemeral_key
    Members         map[string]*Member
    KeyRotationCount int
    LastRotation    time.Time
}

type Member struct {
    Username    string
    PublicKey   []byte
    JoinedAt    time.Time
}
```

### Security Properties
1. **Forward Secrecy**: Ephemeral keys deleted after use
2. **Post-Compromise Security**: Group re-keying excludes compromised keys
3. **Deniability**: No long-term signatures (like Signal)
4. **Confidentiality**: Only group members can decrypt
5. **Integrity**: ChaCha20-Poly1305 provides authentication

## Implementation Phases

### Phase 1: Basic Group Encryption (Week 1)
- [ ] Implement X25519 key exchange
- [ ] Add ChaCha20-Poly1305 encryption
- [ ] Basic group key derivation
- [ ] Encrypt/decrypt group messages

### Phase 2: Forward Secrecy (Week 2)
- [ ] Ephemeral key generation per message
- [ ] Key rotation mechanism
- [ ] Secure key deletion
- [ ] Message replay protection

### Phase 3: Member Management (Week 3)
- [ ] Secure member join protocol
- [ ] Member leave re-keying
- [ ] Key compromise recovery
- [ ] Audit logging

### Phase 4: Optimizations (Week 4)
- [ ] Key caching strategies
- [ ] Batch key operations
- [ ] Performance monitoring
- [ ] Security testing

## Security Considerations

### Threats Mitigated
- **Passive Eavesdropping**: All messages encrypted
- **Key Compromise**: Forward secrecy limits exposure
- **Member Compromise**: Re-keying excludes compromised members
- **Replay Attacks**: Message IDs and timestamps prevent replay

### Threats NOT Mitigated
- **Endpoint Compromise**: If device is compromised, current messages visible
- **Metadata Leakage**: Message timing and sizes still visible
- **Denial of Service**: Malicious re-keying can disrupt service
- **Social Engineering**: Cannot protect against user-level attacks

## Alternatives Considered

1. **No Encryption**: Rejected due to privacy requirements
2. **Simple Shared Key**: Rejected due to lack of forward secrecy
3. **Full Signal Protocol**: Rejected due to implementation complexity
4. **Full MLS with TreeKEM**: Rejected due to specification complexity and newness
   - TreeKEM provides O(log n) re-keying efficiency vs our O(n) approach
   - Better for large groups (100+ members) but overkill for typical chat rooms
   - Could be future enhancement if we need to scale to enterprise-size groups
5. **Pure Diffie-Hellman**: Rejected due to limited forward secrecy

## Consequences

### Positive
- Strong security properties for group messaging
- Forward secrecy protects past communications
- Scalable to reasonable group sizes (< 100 members)
- Industry-standard cryptographic primitives

### Negative
- Increased implementation complexity
- Performance overhead for encryption/decryption
- Key management state to maintain
- Potential for cryptographic bugs

## References
- [Signal Protocol Documentation](https://signal.org/docs/)
- [MLS RFC 9420](https://datatracker.ietf.org/doc/rfc9420/)
- [TreeKEM Paper](https://eprint.iacr.org/2017/1083.pdf)
- [Double Ratchet Algorithm](https://signal.org/docs/specifications/doubleratchet/)
- [X3DH Key Agreement](https://signal.org/docs/specifications/x3dh/)

## Decision Date
2025-01-26

## Decision Makers
- Development Team
- Security Review Team
