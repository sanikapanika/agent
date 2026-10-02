// Sets up a fresh Uptime Kuma (admin user) and adds HTTP monitors FROM..TO
// against the benchmark target, the way Kuma's UI does (over socket.io;
// Kuma has no REST API for this).
//   node kuma.cjs http://bench-kuma:3001 1 50
const { io } = require("socket.io-client");
const [url, from, to] = [process.argv[2], Number(process.argv[3]), Number(process.argv[4])];
const s = io(url, { transports: ["websocket"], reconnection: false });
setTimeout(() => { console.error("timed out"); process.exit(3); }, 300000);
const call = (ev, ...args) => new Promise((res) => s.emit(ev, ...args, res));
// Kuma attaches its handlers only after sending "info"; anything sent
// before that is dropped, so wait for it like Kuma's own UI does.
s.once("info", async () => {
  if (await call("needSetup")) await call("setup", "admin", "bench-pass-123");
  const login = await call("login", { username: "admin", password: "bench-pass-123", token: "" });
  if (!login.ok) { console.error("login failed", login); process.exit(1); }
  for (let i = from; i <= to; i++) {
    const r = await call("add", {
      type: "http", name: `Check ${i}`, url: `http://bench-target/?m=${i}`, method: "GET",
      interval: 60, retryInterval: 60, resendInterval: 0, maxretries: 0, timeout: 10, maxredirects: 10,
      accepted_statuscodes: ["200-299"], notificationIDList: {}, ignoreTls: false, upsideDown: false,
      expiryNotification: false, kafkaProducerBrokers: [], kafkaProducerSaslOptions: { mechanism: "None" },
      conditions: [], rabbitmqNodes: [], active: true,
    });
    if (!r.ok) { console.error("add", i, r); process.exit(1); }
  }
  console.log(`${url}: monitors ${from}-${to} added`);
  process.exit(0);
});
