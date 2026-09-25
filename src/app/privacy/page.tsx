import { PolicyPage } from "@/components/PolicyPage";

export const metadata = {
  title: "VibeGrid privacy",
  description: "What VibeGrid stores for private crew play and reliability."
};

export default function PrivacyPage() {
  return (
    <PolicyPage
      eyebrow="Privacy"
      title="VibeGrid privacy"
      intro="VibeGrid uses guest browser identities. It stores only what is needed to run private crew rounds, preserve their history, and keep the service reliable."
      sections={[
        {
          title: "Browser identity",
          body: [
            "VibeGrid does not require a public account. An HttpOnly guest cookie is a revocable browser key mapped to a stable anonymous identity, so the browser can create or join crews and return to its own cards and votes. The service stores a one-way hash of that key, not the key itself.",
            "You can replace the browser key from a crew's Manage panel if you think it was copied; the previous key stops working while your memberships and history stay attached to the same anonymous identity. This is not account recovery: clearing all browser data or losing the only device still loses access.",
            "Daily and Unlimited practice boards are fetched from the public service, but your practice selections, titles, and tutorial votes stay in the browser and are not submitted. The browser also remembers that its first-visit introduction was dismissed; that preference contains no card or crew content."
          ]
        },
        {
          title: "Crew rounds",
          body: [
            "For each crew, VibeGrid stores the crew name, invite code, member display names, daily card titles, the four selected fragment ids, blind votes, and timestamps.",
            "A card keeps a snapshot of its author display name so an old result remains understandable after membership changes. A crew-specific identity mapping survives leave or removal so rejoining cannot reset the one-card and one-ballot rules. Crew content is returned only to current members; invite links should be treated as private capability links."
          ]
        },
        {
          title: "Reminders",
          body: [
            "Reminders are off unless you turn them on from a crew page, and the browser only asks for notification permission after that click. When they are on, VibeGrid stores the push address your browser's push service issued, the two encryption keys your browser supplied for it, when a reminder last arrived, and a delivery failure count. They are attached to the same anonymous identity as your crews, not to a name, email, or phone number.",
            "A reminder is a daily digest per crew, sent only when something is waiting for you: a result from a round you played, a ballot you have not cast, or today's card. It contains the crew name, a short status line or the day's prompt, and a link to the crew room.",
            "Delivery goes through your browser vendor's push service. The message is encrypted to your browser, but that service can see that a message was sent to your push address and when. Turning reminders off deletes the stored address; so does the push service reporting it no longer exists."
          ]
        },
        {
          title: "Control and retention",
          body: [
            "A crew owner can rotate a leaked invite and remove a member. A member can leave. When the last member chooses the separately confirmed delete-and-leave action, the crew, invite, identity mappings, cards, votes, and result history are permanently deleted through database cascades.",
            "Revoked browser-key hashes are retained as revocation records so an old copied key cannot become valid again. They contain no plaintext bearer value.",
            "Otherwise crew history is retained so members can revisit results. There is no claim here that a provider backup or restore schedule exists until that external configuration is verified."
          ]
        },
        {
          title: "Reliability and safety",
          body: [
            "The service may process IP address, request path, status code, latency, user agent, and rate-limit counters in logs or monitoring systems.",
            "Guest crew writes and admin cookie writes require explicit CSRF tokens. Admin access uses its own revocable HttpOnly session. Public practice does not require a login, email address, or phone number."
          ]
        }
      ]}
    />
  );
}
