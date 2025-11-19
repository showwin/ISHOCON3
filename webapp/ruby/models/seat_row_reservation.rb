# frozen_string_literal: true

class SeatRowReservation < ActiveRecord::Base
  attribute :id, :integer
  attribute :train_id, :integer
  attribute :schedule_id, :string
  attribute :from_station_id, :string
  attribute :to_station_id, :string
  attribute :seat_row, :integer
  attribute :a_is_available, :boolean
  attribute :b_is_available, :boolean
  attribute :c_is_available, :boolean
  attribute :d_is_available, :boolean
  attribute :e_is_available, :boolean
end
