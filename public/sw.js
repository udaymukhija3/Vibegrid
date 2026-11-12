// VibeGrid reminder worker.
//
// The product has no accounts and no address to reach anyone at, and a round
// does not pay out until two days after you play it. This worker is the only
// thing that can bring a crew back, so it stays deliberately small: show what
// the server sent, and open the crew room when it is tapped.

self.addEventListener("install", () => {
  // Take over immediately; there is no cached app shell to keep consistent.
  self.skipWaiting();
});

self.addEventListener("activate", (event) => {
  event.waitUntil(self.clients.claim());
});

self.addEventListener("push", (event) => {
  let payload = {};
  try {
    payload = event.data ? event.data.json() : {};
  } catch (error) {
    payload = {};
  }

  const title = typeof payload.title === "string" && payload.title ? payload.title : "VibeGrid";
  const body = typeof payload.body === "string" ? payload.body : "";
  // The tag collapses repeats: a crew that gets a reminder two days running
  // replaces the old notification instead of stacking another one up.
  const tag = typeof payload.tag === "string" && payload.tag ? payload.tag : "vibegrid";
  const url = typeof payload.url === "string" ? payload.url : "/crews";

  event.waitUntil(
    self.registration.showNotification(title, {
      body,
      tag,
      icon: "/vibegrid-mark.svg",
      badge: "/vibegrid-mark.svg",
      data: { url }
    })
  );
});

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const target = (event.notification.data && event.notification.data.url) || "/crews";

  // Reuse an open tab when there is one, so tapping a reminder does not leave a
  // trail of duplicate VibeGrid tabs behind it.
  event.waitUntil(
    self.clients.matchAll({ type: "window", includeUncontrolled: true }).then((clientList) => {
      for (const client of clientList) {
        if (client.url === target && "focus" in client) {
          return client.focus();
        }
      }
      return self.clients.openWindow ? self.clients.openWindow(target) : undefined;
    })
  );
});
