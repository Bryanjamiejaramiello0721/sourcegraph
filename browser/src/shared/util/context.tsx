import { Observable, of } from 'rxjs'
import { map } from 'rxjs/operators'
import { observeStorageKey } from '../../browser/storage'

export const DEFAULT_SOURCEGRAPH_URL = 'https://sourcegraph.com'

export function observeSourcegraphURL(isExtension: boolean): Observable<string> {
    if (isExtension) {
        return observeStorageKey('sync', 'sourcegraphURL').pipe(
            map(sourcegraphURL => sourcegraphURL || DEFAULT_SOURCEGRAPH_URL)
        )
    }
    return of(window.SOURCEGRAPH_URL || window.localStorage.getItem('SOURCEGRAPH_URL') || DEFAULT_SOURCEGRAPH_URL)
}

/**
 * Returns the base URL where assets will be fetched from
 * (CSS, extension host worker, bundle...).
 *
 * The returned URL is guaranteed to have a trailing slash.
 *
 * If `window.SOURCEGRAPH_ASSETS_URL` is defined by a code host
 * self-hosting the integration bundle, it will be returned.
 * Otherwise, the given `sourcegraphURL` will be used.
 */
export function getAssetsURL(sourcegraphURL: string): string {
    const assetsURL = window.SOURCEGRAPH_ASSETS_URL || new URL('/.assets/extension/', sourcegraphURL).href
    return assetsURL.endsWith('/') ? assetsURL : assetsURL + '/'
}

export function isSourcegraphDotCom(url: string): boolean {
    return url === DEFAULT_SOURCEGRAPH_URL
}

type PlatformName = 'phabricator-integration' | 'bitbucket-integration' | 'firefox-extension' | 'chrome-extension'

export function getPlatformName(): PlatformName {
    if (window.SOURCEGRAPH_PHABRICATOR_EXTENSION) {
        return 'phabricator-integration'
    }
    if (window.SOURCEGRAPH_INTEGRATION) {
        return window.SOURCEGRAPH_INTEGRATION
    }
    return isFirefoxExtension() ? 'firefox-extension' : 'chrome-extension'
}

export function getExtensionVersion(): string {
    if (globalThis.browser) {
        const manifest = browser.runtime.getManifest()
        return manifest.version
    }

    return 'NO_VERSION'
}

function isFirefoxExtension(): boolean {
    return window.navigator.userAgent.includes('Firefox')
}
