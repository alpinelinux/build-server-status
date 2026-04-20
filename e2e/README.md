Run the smallest E2E smoke test like this:

```bash
make e2e
```

This stack starts a local Mosquitto broker on port `1883`, points the backend at it with `BSS_MQTT_BROKER`, and the Playwright test publishes a fixture message over MQTT.
