 Summary: AT_CHECKCODE Is NOT Required for TS.43

  After investigating https://www.rfc-editor.org/rfc/rfc4187.html, https://www.rfc-editor.org/rfc/rfc5448, and how TS.43 works, I've concluded that AT_CHECKCODE is not needed for our implementation.

  What AT_CHECKCODE Does

  AT_CHECKCODE (type 86) is a SHA-1 hash computed over all EAP-Request/AKA-Identity and EAP-Response/AKA-Identity packets exchanged during authentication. Its purpose is to prevent a man-in-the-middle from
  modifying identity information during the identity negotiation phase.

  From RFC 4187 Section 10.13:
  - The checkcode is a 20-byte SHA-1 hash over identity packets
  - If no identity messages were exchanged, AT_CHECKCODE has an empty value (just 2 reserved bytes)
  - It's optional — the server "MAY include" it in Challenge packets, and the peer "MAY include" it in responses

  Why We Don't Need It

  Our TS.43 implementation skips the EAP-Identity exchange entirely:

  Standard EAP-AKA (RFC 4187):
    1. EAP-Request/Identity → EAP-Response/Identity (IMSI)
    2. EAP-Request/AKA-Identity → EAP-Response/AKA-Identity (optional)
    3. EAP-Request/AKA-Challenge → EAP-Response/AKA-Challenge
    4. EAP-Success

  TS.43 (our implementation):
    1. HTTP POST with imsi in body
    2. 401 + EAP-Request/AKA-Challenge (eap_relay)
    3. HTTP POST with EAP-Response/AKA-Challenge (eap_relay)
    4. 200 + EAP-Success + entitlements

  The IMSI comes from the HTTP request body (body.imsi), not from EAP-Identity packets. We go directly to AKA-Challenge without any identity exchange. Looking at our code:

  // src/auth/eapAka.ts:59-80
  export async function handleInitialRequest(
    imsi: string,  // <-- IMSI comes from HTTP body, not EAP-Identity
    clientIp: string,
  ): Promise<ChallengeResult> {
    const vectors = await fetchVectors(imsi);
    // ... derive keys ...
    const challengePacket: EapPacket = {
      subtype: AKA_SUBTYPE.CHALLENGE,  // <-- Go directly to Challenge
      // ...
    };

  We define AKA_SUBTYPE.IDENTITY: 5 but never use it — we never send or receive identity packets.

  The Bidding Down Attack Context

  The "bidding down attack" that AT_CHECKCODE helps prevent is specifically about:
  1. An attacker modifying identity packets to change which authentication method gets negotiated
  2. EAP-AKA' vs EAP-AKA method selection (covered more thoroughly in https://datatracker.ietf.org/doc/rfc9048/)

  Since we don't negotiate methods via EAP-Identity and the IMSI is provided out-of-band (HTTP), this attack vector doesn't apply.

  What If a Device Sends AT_CHECKCODE?

  Our current codec handles unknown attributes via the default case — it would parse AT_CHECKCODE as generic bytes. We wouldn't validate it, but we also wouldn't break. This is acceptable because:
  1. AT_CHECKCODE is optional
  2. A conforming device wouldn't expect us to validate it if we didn't send one ourselves

  Recommendation

  No action needed. AT_CHECKCODE is:
  - Optional per RFC 4187
  - Designed to protect identity packets we don't exchange
  - Not relevant to TS.43's HTTP-based IMSI delivery

  If we wanted to be paranoid, we could add the attribute type constant for documentation purposes:
  AT_CHECKCODE: 86,  // Optional, protects identity exchange (not used in TS.43)

  But implementing actual checkcode generation/verification would be unnecessary complexity for no benefit.

  ---
  Sources:
  - https://www.rfc-editor.org/rfc/rfc4187.html
  - https://www.rfc-editor.org/rfc/rfc5448
  - https://datatracker.ietf.org/doc/rfc9048/
  - https://www.iana.org/assignments/eapsimaka-numbers/eapsimaka-numbers.xhtml
  - https://www.gsma.com/get-involved/working-groups/gsma_resources/ts-43-service-entitlement-configuration/
