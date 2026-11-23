# Hardware security keys for endorsed messages

A direction and not a decision. Nothing here is built, and no record depends on it. A part becomes an ADR when someone
takes it on. Set down on 2026-11-23.

## Endorsed messages

Endorsed messages are a planned record. A person approves a Landline message with a presence prompt from the system:
Touch ID, an Apple Watch, or the login password. The person picks which ones count with a setting of `any`, `password`,
or `biometric`.

## A hardware key as a presence method

A later version may accept a hardware security key, such as a YubiKey, as one more presence method. A touch on the key
would show that a person was there, as a fingerprint does.

It is deferred because it cannot be tested yet. The first version of endorsed messages leaves it out.
