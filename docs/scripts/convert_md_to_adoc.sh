# Create input.md file, paste markdown text, and run script. The output will be printed to the output.adoc file.
pandoc --from=gfm --to=asciidoc --wrap=none --atx-headers  input.md > output.adoc
