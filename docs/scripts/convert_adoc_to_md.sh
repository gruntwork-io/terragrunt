# Required:
#  - asciidoctor
#  - pandoc
#
# Install Asciidoctor:
# $ sudo apt-get install asciidoctor
#
# Install Pandoc
# https://pandoc.org/installing.html
#
# 1. Create input.adoc file
# 2. paste adoc-formatted content to input.adoc
# 3. run script
# 4. The output will be printed to the output.md file.
asciidoctor -b docbook input.adoc && pandoc -f docbook -t gfm input.xml -o output.md --wrap=none --atx-headers
