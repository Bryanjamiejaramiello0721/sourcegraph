# LSIF worker jobs

The following job types are enqueued into [node-resque](https://github.com/taskrabbit/node-resque) and handled by the an instance of the worker process. Each job is enqueued with its `class` (the job type) and its `args` (a positional sequence of values).

### `convert(repository, commit, filepath)`

Convert an LSIF dump into a SQLite database and add cross-repository information into the cross-repository database (defined packages, imported references, and an LSIF data marker).

The repository and commit arguments denote the name of the repository and the commit from which the LSIF upload was generated. The filepath argument denotes the path on disk where the LSIF upload data can be found. The file contents are assumed to be gzipped, and each line of the file contains a vertex or edge structure encoded as JSON.
