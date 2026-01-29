# pkg

Packages in the `/pkg` directory are used across the application, and are potentially 
useful for reuse outside the repository.

Practically speaking, this directory is a set of libraries that other Gruntwork
projects might benefit from. There is no stability guarantee for the packages in this
directory, and they are not guaranteed to be stable from one release to the next.

When a project is considered stable, and generally useful for consumption outside
Gruntwork, a dedicated repository should be created for it and it should be moved
out of this directory.
