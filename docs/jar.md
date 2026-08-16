# jar info — Analyze JAR files

Parse class file versions, MANIFEST.MF, Maven coordinates, and multi-release info from a JAR.
Supports CLI output and web UI (file upload).

```bash
# CLI analysis
mu jar info app.jar

# Serve web UI (standalone)
mu jar info serve --port 8086
```

```
$ mu jar info app.jar
Target JDK:     11
Classes:        342
Total entries:  512
Compressed:     1.2 MB → 2.8 MB (43%)
Manifest:
  Main-Class:            com.example.Main
  Created-By:            Apache Maven 3.9.6
  Build-Jdk:             17.0.8
  Implementation-Version: 2.1.0
  Automatic-Module-Name:  com.example.myapp
Maven:          com.example:my-app:1.2.3
Signed:         false
Multi-release:  true
  JDK 9:  8 classes
  JDK 11: 12 classes
Version breakdown:
  Java 8  (52):   322
  Java 11 (55):   20
```
