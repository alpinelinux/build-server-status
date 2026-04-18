const { test, expect } = require('@playwright/test');
const mqtt = require('mqtt');

function publish(topic, payload) {
  return new Promise((resolve, reject) => {
    const client = mqtt.connect('mqtt://127.0.0.1:1883');

    client.once('error', err => {
      client.end(true);
      reject(err);
    });

    client.once('connect', () => {
      client.publish(topic, payload, { qos: 1 }, err => {
        if (err) {
          client.end(true);
          reject(err);
          return;
        }
        setTimeout(() => {
          client.end(true);
          resolve();
        }, 200);
      });
    });
  });
}

test('renders a builder row for an MQTT event', async ({ page }) => {
  await page.goto('/');

  await expect(page.locator('#mqtt_connect_status')).toContainText('CONNECTED');

  await publish('build/test-builder-x86_64', 'pulling git');

  await expect(page.locator('#servers')).toContainText('test-builder-x86_64');
  await expect(page.locator('#servers')).toContainText('pulling git');
});
