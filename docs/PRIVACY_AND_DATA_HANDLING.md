# Privacy and data handling

Telegram and the hosting provider see their own account, delivery and
connection metadata. The service hides participants’ direct identifiers
from each other during normal relay. Authorized operators can access
session history and media in the panel. Moderation rules can miss contact
information, including text embedded in images or spoken in an audio file.

PostgreSQL stores participant routing, session state, message text, delivery
status and operator audit. Participant names and usernames in the user
repository use the deployment encryption key. Incoming requests have a
separate record and may expose submitter details or initial text to
operators, including through the configured operator group. Redis holds
temporary authentication state and moderation counters. Media volumes
hold files used for delivery and preview.

Unconfirmed TOTP enrollment uses a short-lived encrypted Redis value.
Confirmed operator TOTP secrets are encrypted in PostgreSQL using a key
derived from the deployment encryption key. Legacy plaintext records
upgrade on successful TOTP verification.

The retention process applies configured cutoffs to local data and media.
Defaults and operational checks are in [Retention](RETENTION.md).
Telegram copies, disk snapshots and exported audit information have
separate lifecycles. Access and expiry must be managed in each storage
location. A restore requires the matching separately protected encryption
key to read encrypted participant fields.
