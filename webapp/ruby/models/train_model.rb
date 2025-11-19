# frozen_string_literal: true

class TrainModel < ActiveRecord::Base
  attribute :name, :string
  attribute :seat_rows, :integer
  attribute :seat_columns, :integer
end
