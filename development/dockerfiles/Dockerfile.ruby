FROM ruby:3.2

WORKDIR /home/ishocon/webapp/ruby

RUN apt-get update && apt-get install --no-install-recommends -y \
    default-mysql-client-core \
 && apt-get clean \
 && rm -rf /var/lib/apt/lists/*

RUN gem install bundler

COPY Gemfile Gemfile.lock ./

RUN bundle install # --without development test

COPY . .

EXPOSE 8080

CMD ["bundle", "exec", "puma", "-b", "tcp://0.0.0.0:8080"]
