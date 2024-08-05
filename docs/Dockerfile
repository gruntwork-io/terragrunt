FROM ruby:3.3-bookworm
MAINTAINER Gruntwork <info@gruntwork.io>

# Copy the Gemfile and Gemfile.lock into the image and run bundle install in a way that will be cached
WORKDIR /tmp
ADD Gemfile Gemfile
ADD Gemfile.lock Gemfile.lock
RUN bundle install

RUN mkdir -p /src
VOLUME ["/src"]
WORKDIR /src
COPY . /src

# Jekyll runs on port 4000 by default
EXPOSE 4000

# Run jekyll serve - jekyll will build first to create a plain html file for TOS update
CMD ["./jekyll-serve.sh"]
