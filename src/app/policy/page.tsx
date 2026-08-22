import { PolicyPage } from "@/components/PolicyPage";

export const metadata = {
  title: "VibeGrid crew rules",
  description: "The rules for titles, display names, and private crew play."
};

export default function CrewPolicyPage() {
  return (
    <PolicyPage
      eyebrow="Crew policy"
      title="Keep the joke inside the room."
      intro="VibeGrid works when a crew can be specific, funny, and honest without turning a card into a weapon. These are the boundaries."
      sections={[
        {
          title: "Hard lines",
          body: [
            "Do not use a display name or card title for threats, hate, targeted harassment, sexual content involving minors, impersonation, or disclosure of private information.",
            "Do not turn an invite into a public pile-on. The fact that a room is link-accessible does not make its people or its history public material."
          ]
        },
        {
          title: "The social contract",
          body: [
            "Vote for the card, not against its eventual author. The ballot hides authors for a reason, and self-votes are blocked.",
            "Inside jokes are welcome when everyone in the room can laugh. A private reference designed to humiliate one member is not."
          ]
        },
        {
          title: "What owners can do",
          body: [
            "Owners can remove a member or rotate a leaked invite. Members can leave at any time. These controls change access going forward; they do not silently rewrite an old revealed result.",
            "There is not yet a public in-product report queue for crew cards. If that becomes part of the product, the privacy and moderation documentation must be updated before launch."
          ]
        }
      ]}
    />
  );
}
