export class RefCount<Key = string> {
    private refCount = new Map<Key, number>()

    /**
     * Increment the refCount for the given key.
     *
     * @returns true if the given key is new.
     */
    public increment(key: Key): boolean {
        const current = this.refCount.get(key)
        if (current === undefined) {
            this.refCount.set(key, 1)
            return true
        }
        this.refCount.set(key, current + 1)
        return false
    }

    /**
     * Decrements the refCount for the given key.
     *
     * @returns true if this was the given key's last reference.
     */
    public decrement(key: Key): boolean {
        const current = this.refCount.get(key)
        if (current === undefined) {
            throw new Error(`No refCount for key: ${key}`)
        } else if (current === 1) {
            this.refCount.delete(key)
            return true
        } else {
            this.refCount.set(key, current - 1)
            return false
        }
    }

    /**
     * Returns an iterable of registered keys.
     */
    public keys(): Iterable<Key> {
        return this.refCount.keys()
    }

    public delete(key: Key): void {
        this.refCount.delete(key)
    }
}
