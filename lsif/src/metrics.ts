import promClient from 'prom-client'

//
// Database Metrics

export const databaseQueryDurationHistogram = new promClient.Histogram({
    name: 'lsif_database_query_duration_seconds',
    help: 'Total time spent on database queries.',
    buckets: [0.2, 0.5, 1, 2, 5, 10, 30],
})

export const xrepoQueryDurationHistogram = new promClient.Histogram({
    name: 'lsif_xrepo_query_duration_seconds',
    help: 'Total time spent on cross-repo database queries.',
    buckets: [0.2, 0.5, 1, 2, 5, 10, 30],
})

export const databaseQueryErrorsCounter = new promClient.Counter({
    name: 'lsif_database_query_errors_total',
    help: 'The number of errors that occurred during a database query.',
})

export const xrepoQueryErrorsCounter = new promClient.Counter({
    name: 'lsif_xrepo_query_errors_total',
    help: 'The number of errors that occurred during a cross-repo database query.',
})

//
// Database Insertion Metrics

export const databaseInsertionDurationHistogram = new promClient.Histogram({
    name: 'lsif_database_insertion_duration_seconds',
    help: 'Total time spent on database insertions.',
    buckets: [0.2, 0.5, 1, 2, 5, 10, 30],
})

export const xrepoInsertionDurationHistogram = new promClient.Histogram({
    name: 'lsif_xrepo_insertion_duration_seconds',
    help: 'Total time spent on cross-repo database insertions.',
    buckets: [0.2, 0.5, 1, 2, 5, 10, 30],
})

export const databaseInsertionErrorsCounter = new promClient.Counter({
    name: 'lsif_database_insertion_errors_total',
    help: 'The number of errors that occurred during a database insertion.',
})

export const xrepoInsertionErrorsCounter = new promClient.Counter({
    name: 'lsif_xrepo_insertion_errors_total',
    help: 'The number of errors that occurred during a cross-repo database insertion.',
})

//
// Cache Metrics

export const connectionCacheCapacityGauge = new promClient.Gauge({
    name: 'lsif_connection_cache_capacity',
    help: 'The maximum number of open SQLite handles.',
})

export const connectionCacheSizeGauge = new promClient.Gauge({
    name: 'lsif_connection_cache_size',
    help: 'The current number of open SQLite handles.',
})

export const connectionCacheEventsCounter = new promClient.Counter({
    name: 'lsif_connection_cache_events_total',
    help: 'The number of connection cache hits, misses, and evictions.',
    labelNames: ['type'],
})

export const documentCacheCapacityGauge = new promClient.Gauge({
    name: 'lsif_document_cache_capacity',
    help: 'The maximum number of documents loaded in memory.',
})

export const documentCacheSizeGauge = new promClient.Gauge({
    name: 'lsif_document_cache_size',
    help: 'The current number of documents loaded in memory.',
})

export const documentCacheEventsCounter = new promClient.Counter({
    name: 'lsif_document_cache_events_total',
    help: 'The number of document cache hits, misses, and evictions.',
    labelNames: ['type'],
})

export const resultChunkCacheCapacityGauge = new promClient.Gauge({
    name: 'lsif_results_chunk_cache_capacity',
    help: 'The maximum number of result chunks loaded in memory.',
})

export const resultChunkCacheSizeGauge = new promClient.Gauge({
    name: 'lsif_results_chunk_cache_size',
    help: 'The current number of result chunks loaded in memory.',
})

export const resultChunkCacheEventsCounter = new promClient.Counter({
    name: 'lsif_results_chunk_cache_events_total',
    help: 'The number of result chunk cache hits, misses, and evictions.',
    labelNames: ['type'],
})

//
// Bloom Filter Metrics

export const bloomFilterEventsCounter = new promClient.Counter({
    name: 'lsif_bloom_filter_events_total',
    help: 'The number of bloom filter hits and misses.',
    labelNames: ['type'],
})

//
// Helpers

export async function instrument<T>(
    durationHistogram: promClient.Histogram,
    errorsCounter: promClient.Counter,
    fn: () => Promise<T>
): Promise<T> {
    const end = durationHistogram.startTimer()
    try {
        return await fn()
    } catch (e) {
        errorsCounter.inc()
        throw e
    } finally {
        end()
    }
}
