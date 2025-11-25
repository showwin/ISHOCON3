# frozen_string_literal: true

class TrainSchedule < ActiveRecord::Base
  attribute :id, :string
  attribute :train_id, :integer
  attribute :departure_at_station_a_to_b, :string
  attribute :departure_at_station_b_to_c, :string
  attribute :departure_at_station_c_to_d, :string
  attribute :departure_at_station_d_to_e, :string
  attribute :departure_at_station_e_to_d, :string
  attribute :departure_at_station_d_to_c, :string
  attribute :departure_at_station_c_to_b, :string
  attribute :departure_at_station_b_to_a, :string
end
