# Moderation

The backend checks relay text and photo or voice captions before saving
them for delivery. It also checks operator send-as text. Rules recognize
email, known messenger links, handles, phone formats, and Russian and
English phrases that suggest moving off-platform. Selected bracketed
email obfuscations, full-width ASCII and invisible format characters are
normalized for matching. The delivered content remains unchanged.

By default a matching message is blocked and the operator group receives
a notice. The session stays active. Set `MODERATION_AUTO_PAUSE=true` to
pause following three violations by the same participant in a session.
The Redis window is 24 hours per session with separate actor counters.
Resuming a session clears its violation counters. If Redis cannot record a
violation, the message is rejected and the bot prompts the sender to retry.

`MODERATION_DISABLED_RULES` accepts a comma-separated list of
`email`, `messenger_link`, `username`, `phone` and `suspicious_phrase`.
An empty list enables all rules. Unknown names fail startup.

The [synthetic corpus](../backend/internal/moderation/testdata/corpus.tsv)
has 161 labelled cases. A local run measured 79 true positives, 75 true
negatives, 5 false positives and 2 false negatives. Precision was 94.05%
and recall was 97.53% on this corpus. These figures characterize that test
set. Quoted contacts and some phone-like numeric references can trigger
rules. Unrecognized obfuscation, images and speech can pass.
