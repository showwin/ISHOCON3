# frozen_string_literal: true

require 'net/http'

module PaymentApp
  module_function

  HOST = ENV.fetch('ISHOCON_PAYMENT_HOST', 'payment_app')
  PORT = ENV.fetch('ISHOCON_PAYMENT_PORT', 8081)

  def initialize
    uri = URI("http://#{HOST}:#{PORT}/initialize")
    res = Net::HTTP.post(uri, '')

    raise PaymentAppInitializationFailed, "Failed to initialize payment app: #{res.code}" unless res.is_a?(Net::HTTPSuccess)
  end

  def capture_payment(amount, token)
    uri = URI("http://#{HOST}:#{PORT}/payments")

    Net::HTTP.post(uri, { amount: amount, global_payment_token: token }.to_json)
  end

  class PaymentAppInitializationFailed < StandardError; end
end
