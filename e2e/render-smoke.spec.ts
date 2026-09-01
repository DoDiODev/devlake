/*
Licensed to the Apache Software Foundation (ASF) under one or more
contributor license agreements.  See the NOTICE file distributed with
this work for additional information regarding copyright ownership.
The ASF licenses this file to You under the Apache License, Version 2.0
(the "License"); you may not use this file except in compliance with
the License.  You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

import { test, expect } from '@playwright/test';

const UI = 'http://localhost:4000';

/**
 * Smoke test for the React 19 upgrade regressions.
 *
 * Requires the config-ui dev server running on :4000 (`yarn start`). The backend
 * does not need to be up — API calls may 502, which is fine; this test only
 * asserts the SPA itself mounts and that the two React-19 crashes are gone:
 *   1. miller-columns-select reading removed internals (ReactCurrentDispatcher).
 *   2. the entry point calling the removed ReactDOM.render API.
 */
test.describe('Config-UI render smoke (React 19 regressions)', () => {
  test('app mounts without React runtime crashes', async ({ page }) => {
    const consoleErrors: string[] = [];
    const pageErrors: string[] = [];

    page.on('console', (msg) => {
      if (msg.type() === 'error') consoleErrors.push(msg.text());
    });
    page.on('pageerror', (err) => pageErrors.push(err.message));

    await page.goto(UI, { waitUntil: 'domcontentloaded' });

    // The SPA must actually render into #root (a white screen leaves it empty).
    await page.waitForFunction(
      () => {
        const root = document.getElementById('root');
        return !!root && root.childElementCount > 0;
      },
      { timeout: 30000 },
    );

    const rootHtml = (await page.locator('#root').innerHTML()).trim();
    expect(rootHtml.length).toBeGreaterThan(0);

    const allErrors = [...consoleErrors, ...pageErrors];

    // Regression 1: miller-columns-select bundled jsx-runtime hit removed React 19 internals.
    expect(allErrors.find((e) => e.includes('ReactCurrentDispatcher'))).toBeUndefined();

    // Regression 2: entry used the removed ReactDOM.render API.
    expect(allErrors.find((e) => /ReactDOM\.render is not a function/.test(e))).toBeUndefined();
  });
});
