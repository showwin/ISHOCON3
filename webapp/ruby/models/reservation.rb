# frozen_string_literal: true

class Reservation < ActiveRecord::Base
  attribute :id, :string
  attribute :user_id, :string
  attribute :schedule_id, :string
  attribute :from_station_id, :string
  attribute :to_station_id, :string
  attribute :departure_at, :string
  attribute :entry_token, :string
  attribute :created_at, :datetime
end
