import crypto from 'node:crypto';

/** Encrypt plaintext with a DEK using AES-256-GCM. Returns IV[12] || ciphertext || authTag[16]. */
export function encryptWithDek(dek: Buffer, plaintext: Buffer): Buffer {
  const iv = crypto.randomBytes(12);
  const cipher = crypto.createCipheriv('aes-256-gcm', dek, iv);
  const ciphertext = Buffer.concat([cipher.update(plaintext), cipher.final()]);
  const authTag = cipher.getAuthTag(); // 16 bytes
  return Buffer.concat([iv, ciphertext, authTag]);
}

/** Decrypt data produced by encryptWithDek. */
export function decryptWithDek(dek: Buffer, encrypted: Buffer): Buffer {
  const iv = encrypted.subarray(0, 12);
  const authTag = encrypted.subarray(encrypted.length - 16);
  const ciphertext = encrypted.subarray(12, encrypted.length - 16);

  const decipher = crypto.createDecipheriv('aes-256-gcm', dek, iv);
  decipher.setAuthTag(authTag);
  return Buffer.concat([decipher.update(ciphertext), decipher.final()]);
}

/** Wrap a DEK using XOR with a KEK (simple dev-only scheme). Both must be 32 bytes. */
export function wrapDek(kek: Buffer, dek: Buffer): Buffer {
  if (kek.length !== 32 || dek.length !== 32) {
    throw new Error('KEK and DEK must both be 32 bytes');
  }
  const wrapped = Buffer.allocUnsafe(32);
  for (let i = 0; i < 32; i++) {
    wrapped[i] = kek[i]! ^ dek[i]!;
  }
  return wrapped;
}

/** Unwrap a DEK using XOR with a KEK. */
export function unwrapDek(kek: Buffer, wrappedDek: Buffer): Buffer {
  return wrapDek(kek, wrappedDek); // XOR is its own inverse
}

/** Zero out a buffer for memory hygiene. */
export function zeroBuffer(buf: Buffer): void {
  buf.fill(0);
}

/** Key manager interface for abstracting key unwrapping. */
export interface KeyManager {
  unwrapDek(wrappedDek: Buffer): Buffer | Promise<Buffer>;
}

/** Local key manager using a KEK from environment (dev only). */
export class LocalKeyManager implements KeyManager {
  private readonly kek: Buffer;

  constructor(kekHex: string) {
    this.kek = Buffer.from(kekHex, 'hex');
    if (this.kek.length !== 32) {
      throw new Error('LOCAL_KEK_HEX must be 64 hex chars (32 bytes)');
    }
  }

  unwrapDek(wrappedDek: Buffer): Buffer {
    return unwrapDek(this.kek, wrappedDek);
  }
}

/**
 * Cloud KMS key manager for production use.
 * Uses Google Cloud KMS to decrypt (unwrap) DEKs via envelope encryption.
 * The wrapped DEK was encrypted with the KMS key, so we call KMS decrypt to recover it.
 */
export class CloudKmsKeyManager implements KeyManager {
  private readonly keyName: string;
  private client: any; // Lazy-loaded @google-cloud/kms client

  constructor(projectId: string, locationId: string, keyRingId: string, keyId: string) {
    this.keyName = `projects/${projectId}/locations/${locationId}/keyRings/${keyRingId}/cryptoKeys/${keyId}`;
  }

  async unwrapDek(wrappedDek: Buffer): Promise<Buffer> {
    if (!this.client) {
      // Lazy import to avoid requiring @google-cloud/kms in dev environments
      const { KeyManagementServiceClient } = await import('@google-cloud/kms');
      this.client = new KeyManagementServiceClient();
    }

    const [result] = await this.client.decrypt({
      name: this.keyName,
      ciphertext: wrappedDek,
    });

    return Buffer.from(result.plaintext as Uint8Array);
  }
}

/** Create the appropriate key manager based on environment configuration. */
export function createKeyManager(config: {
  localKekHex: string;
  gcpProjectId: string;
  kmsLocation: string;
  kmsKeyRing: string;
  kmsKeyName: string;
}): KeyManager {
  if (config.localKekHex) {
    return new LocalKeyManager(config.localKekHex);
  }

  if (!config.gcpProjectId) {
    throw new Error('Either LOCAL_KEK_HEX or GCP_PROJECT_ID must be set');
  }

  return new CloudKmsKeyManager(
    config.gcpProjectId,
    config.kmsLocation,
    config.kmsKeyRing,
    config.kmsKeyName,
  );
}
