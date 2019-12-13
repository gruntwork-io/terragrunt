# Custom less compiler based on: https://gist.github.com/KBalderson/5689220
module Jekyll
  class LessConverter < Converter
    safe true
    priority :high

    def setup
      return if @setup
      require 'less'
      @setup = true
    rescue LoadError
      STDERR.puts 'You are missing the library required for less. Please run:'
      STDERR.puts ' $ [sudo] gem install less'
      raise FatalException.new("Missing dependency: less")
    end

    def matches(ext)
      ext =~ /less|lcss/i
    end

    def output_ext(ext)
      ".css"
    end

    def convert(content)
      setup
      begin
        parser = Less::Parser.new(:paths => ['assets/css'], :compress => true)
        parser.parse(content).to_css(:compress => true)
      rescue => e
        puts "Less Exception: #{e.message}"
      end
    end
  end
end
