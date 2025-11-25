# frozen_string_literal: true

class Payment < ActiveRecord::Base
  attribute :id, :integer
  attribute :user_id, :string
  attribute :reservation_id, :string
  attribute :amount, :integer
  attribute :is_captured, :boolean
  attribute :is_refunded, :boolean
  attribute :created_at, :datetime
  attribute :updated_at, :datetime
end
