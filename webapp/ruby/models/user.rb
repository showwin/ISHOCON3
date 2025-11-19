# frozen_string_literal: true

class User < ActiveRecord::Base
  attribute :id, :string
  attribute :name, :string
  attribute :hashed_password, :string
  attribute :salt, :string
  attribute :is_admin, :boolean
  attribute :global_payment_token, :string
  attribute :last_activity_at, :datetime
  attribute :created_at, :datetime
end
